package com.kinjo.app

import android.Manifest
import android.annotation.SuppressLint
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.bluetooth.BluetoothAdapter
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ServiceInfo
import android.hardware.Sensor
import android.hardware.SensorManager
import android.hardware.TriggerEvent
import android.hardware.TriggerEventListener
import android.location.Location
import android.net.ConnectivityManager
import android.net.Network
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.google.android.gms.location.LocationCallback
import com.google.android.gms.location.LocationRequest
import com.google.android.gms.location.LocationResult
import com.google.android.gms.location.LocationServices
import com.google.android.gms.location.Priority
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONArray
import org.json.JSONObject
import kotlin.random.Random

/**
 * Keeps this phone findable: GPS fixes and Bluetooth hearings go to the server over one socket,
 * and the nearby deck comes back on it. Two modes: moving (fast fixes) and still (a cheap fix every 20 s,
 * woken by the motion sensor). Everything except the socket and sensor callbacks runs on the main thread.
 */
class PresenceService : Service() {

    private val fused by lazy { LocationServices.getFusedLocationProviderClient(this) }
    private val sensors by lazy { getSystemService(SensorManager::class.java) }
    private val ble by lazy { Ble(this) }
    private val main = Handler(Looper.getMainLooper())
    private var running = false
    private var stopped = false
    private var socket: WebSocket? = null
    private var retries = 0
    private var bleToken: String? = null

    private var still = false
    private var anchor: Location? = null // where the phone seems to have settled
    private var anchorSince = 0L
    private var lastFix: Location? = null // resent on reconnect, so the card is back at once

    private val onFix = object : LocationCallback() {
        override fun onLocationResult(result: LocationResult) {
            val fix = result.lastLocation ?: return
            trail(this@PresenceService, describe(fix, still))
            updateMotion(fix)
            lastFix = fix
            socket?.let { send(it, fix) }
        }
    }

    private fun send(socket: WebSocket, fix: Location) = socket.send(
        JSONObject().put("type", "fix").put("lat", fix.latitude).put("lng", fix.longitude)
            .put("acc", fix.accuracy.toDouble()).put("still", still).toString(),
    )

    private val onMotion = object : TriggerEventListener() {
        override fun onTrigger(event: TriggerEvent?) {
            main.post { if (still) setStill(false) }
        }
    }

    private val reportHeard = object : Runnable {
        override fun run() {
            val heard = ble.drain()
            if (heard.isNotEmpty()) {
                val seen = JSONArray()
                heard.forEach { (token, rssi) -> seen.put(JSONObject().put("t", token).put("rssi", rssi)) }
                socket?.send(JSONObject().put("type", "seen").put("seen", seen).toString())
            }
            main.postDelayed(this, if (still) 20_000L else 2_000L)
        }
    }

    private val reconnect = Runnable { connect() }

    // Without the server for this long, the deck may show people who left: clear it.
    private val clearStale = Runnable { nearbyCards.value = emptyList() }

    // Back online: reconnect now instead of waiting out the backoff.
    private val network = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(n: Network) {
            main.post {
                if (stopped || socket != null) return@post
                main.removeCallbacks(reconnect)
                retries = 0
                connect()
            }
        }
    }

    private val listener = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            main.post {
                retries = 0
                main.removeCallbacks(clearStale)
                // Only a recent position: an old one could put us back somewhere we already left.
                lastFix?.takeIf { (SystemClock.elapsedRealtimeNanos() - it.elapsedRealtimeNanos) / 1_000_000 < STILL_FIX_MS + 10_000 }
                    ?.let { send(webSocket, it) }
            }
            trail(this@PresenceService, "socket open")
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            val message = runCatching { JSONObject(text) }.getOrNull() ?: return
            when (message.optString("type")) {
                "deck" -> runCatching { parseCards(message) }.getOrNull()?.let { nearbyCards.value = it }
                "ble" -> message.optString("token").takeIf { TOKEN.matches(it) }?.let { token ->
                    main.post {
                        bleToken = token
                        ble.start(token, fast = !still)
                    }
                }
                "vis" -> if (!message.optBoolean("visible", true)) {
                    // Hidden from another phone or the settings screen: this phone stops reporting at once.
                    Api.rememberVisible(this@PresenceService, false)
                    main.post { stopSelf() }
                }
            }
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) = lost(webSocket, "closed $code")

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            if (response?.code == 401) Api.forget(this@PresenceService)
            lost(webSocket, "failed ${response?.code ?: t.javaClass.simpleName}")
        }
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // A null intent means the OS restarted us after killing the process (START_STICKY).
        val why = intent?.getStringExtra(EXTRA_WHY) ?: "sticky"
        trail(this, "start why=$why")
        try {
            ServiceCompat.startForeground(this, NOTIFICATION_ID, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION)
        } catch (e: SecurityException) { // permission revoked since start() checked it, or a sticky restart without it
            trail(this, "start refused why=$why: ${e.message}")
            stopSelf()
            return START_NOT_STICKY
        }
        if (!running) {
            running = true
            requestFixes()
            main.post(reportHeard)
            getSystemService(ConnectivityManager::class.java)?.registerDefaultNetworkCallback(network)
            ContextCompat.registerReceiver(this, bluetooth, IntentFilter(BluetoothAdapter.ACTION_STATE_CHANGED), ContextCompat.RECEIVER_NOT_EXPORTED)
            ble.switchOn()
            RestartReceiver.watchMovement(this)
        }
        // Android saw walking, or us leaving the resting spot: trust it even if the motion sensor missed it.
        if (still && (why == "moving" || why == "left spot")) setStill(false)
        if (why != "user") Health.report(this) // restarted by Android: why we were stopped is fresh now
        bleToken?.let { ble.start(it, fast = !still) } // a Bluetooth permission or radio switched on since
        connect()
        return START_STICKY
    }

    override fun onTaskRemoved(rootIntent: Intent?) = trail(this, "swiped")

    override fun onDestroy() {
        stopped = true
        main.removeCallbacksAndMessages(null)
        if (running) {
            runCatching { getSystemService(ConnectivityManager::class.java)?.unregisterNetworkCallback(network) }
            runCatching { unregisterReceiver(bluetooth) }
        }
        fused.removeLocationUpdates(onFix)
        sensors?.getDefaultSensor(Sensor.TYPE_SIGNIFICANT_MOTION)?.let { sensors?.cancelTriggerSensor(onMotion, it) }
        ble.stop()
        socket?.close(1000, null)
        socket = null
        nearbyCards.value = emptyList()
        trail(this, "destroyed")
    }

    private fun connect() {
        if (stopped || socket != null) return
        val token = Api.token(this) ?: return trail(this, "no session, socket not opened")
        socket = Api.socket(token, listener)
    }

    // Reconnect after 0.5, 1, 2, 4, then every 5 s, plus up to 1 s at random so a server restart isn't hit by every phone at once.
    private fun lost(webSocket: WebSocket, why: String) {
        main.post {
            if (socket !== webSocket) return@post // already replaced, or closed by onDestroy
            socket = null
            trail(this, "socket $why")
            if (stopped) return@post
            main.removeCallbacks(clearStale)
            main.postDelayed(clearStale, STALE_MS)
            val backoff = minOf(5_000L, 500L shl minOf(retries++, 4))
            main.postDelayed(reconnect, backoff + Random.nextLong(1_000))
        }
    }

    // Bluetooth switched off: turn it back on where Android allows it; switched on: use it at once.
    private val bluetooth = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            when (intent.getIntExtra(BluetoothAdapter.EXTRA_STATE, -1)) {
                BluetoothAdapter.STATE_ON -> bleToken?.let { ble.start(it, fast = !still) }
                BluetoothAdapter.STATE_OFF -> {
                    ble.stop()
                    ble.switchOn()
                }
            }
        }
    }

    // Still = within STILL_RADIUS_M of one spot for STILL_AFTER_MS; leaving that spot means moving again.
    private fun updateMotion(fix: Location) {
        val spot = anchor
        val now = SystemClock.elapsedRealtime()
        when {
            spot == null || fix.distanceTo(spot) > STILL_RADIUS_M + fix.accuracy -> {
                anchor = fix
                anchorSince = now
                if (still) setStill(false)
            }
            !still && now - anchorSince > STILL_AFTER_MS -> setStill(true)
        }
    }

    private fun setStill(value: Boolean) {
        still = value
        trail(this, if (value) "still" else "moving")
        if (!value) anchorSince = SystemClock.elapsedRealtime() // stopping again only counts after a fresh 2 minutes
        if (value) anchor?.let { RestartReceiver.watchSpot(this, it.latitude, it.longitude) } // if we're killed here, leaving revives us
        requestFixes()
        bleToken?.let { ble.start(it, fast = !value) }
        val motion = sensors?.getDefaultSensor(Sensor.TYPE_SIGNIFICANT_MOTION) ?: return
        if (value) sensors?.requestTriggerSensor(onMotion, motion) else sensors?.cancelTriggerSensor(onMotion, motion)
    }

    @SuppressLint("MissingPermission") // startForeground() above throws without it
    private fun requestFixes() {
        val request = if (still) {
            LocationRequest.Builder(Priority.PRIORITY_BALANCED_POWER_ACCURACY, STILL_FIX_MS).build()
        } else {
            LocationRequest.Builder(Priority.PRIORITY_HIGH_ACCURACY, 2_000).setMinUpdateIntervalMillis(1_000).build()
        }
        fused.requestLocationUpdates(request, onFix, Looper.getMainLooper()) // replaces the previous request
    }

    private fun notification(): Notification {
        val channel = NotificationChannel(CHANNEL, "Finding people nearby", NotificationManager.IMPORTANCE_LOW)
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        return NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle("Kinjo is finding people nearby")
            .setContentIntent(open)
            .setOngoing(true)
            .build()
    }

    companion object {
        private const val EXTRA_WHY = "why"
        private const val CHANNEL = "presence"
        private const val NOTIFICATION_ID = 1
        private const val STILL_RADIUS_M = 15f
        private const val STILL_AFTER_MS = 120_000L
        private const val STILL_FIX_MS = 20_000L // resting: a cheap fix every 20 s, so walking off is noticed fast
        private const val STALE_MS = 120_000L
        private val TOKEN = Regex("[0-9a-f]{16}")

        /** Starts presence unless the user is hidden. `why` goes in the trail to show which trigger revived it. */
        fun start(context: Context, why: String) {
            if (!Api.visible(context)) return trail(context, "not started why=$why: hidden")
            // Only a start from the visible app may rely on while-in-use location.
            val needed = if (why == "user") listOf(Manifest.permission.ACCESS_FINE_LOCATION)
            else listOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_BACKGROUND_LOCATION)
            val missing = needed.filterNot { context.granted(it) }
            if (missing.isNotEmpty()) return trail(context, "not started why=$why missing=$missing")
            ContextCompat.startForegroundService(context, Intent(context, PresenceService::class.java).putExtra(EXTRA_WHY, why))
        }

        fun stop(context: Context) = context.stopService(Intent(context, PresenceService::class.java))

        private fun parseCards(message: JSONObject): List<Card> {
            val cards = message.getJSONArray("cards")
            return List(cards.length()) { i ->
                val c = cards.getJSONObject(i)
                Card(c.getString("uid"), c.getString("name"), c.getString("role"), c.getString("looking"), c.getString("img"))
            }
        }

        @Suppress("DEPRECATION") // isMock needs API 31; isFromMockProvider still works on 29-36
        private fun describe(fix: Location, still: Boolean): String {
            val ageMs = (SystemClock.elapsedRealtimeNanos() - fix.elapsedRealtimeNanos) / 1_000_000
            return "fix acc=${fix.accuracy.toInt()}m age=${ageMs}ms mock=${fix.isFromMockProvider} still=$still"
        }
    }
}
