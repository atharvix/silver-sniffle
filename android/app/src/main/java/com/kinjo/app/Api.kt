package com.kinjo.app

import android.content.Context
import android.content.pm.PackageManager
import android.util.Base64
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.random.Random

/** A nearby person's card, exactly as the server sent it. */
data class Card(val uid: String, val name: String, val role: String, val looking: String, val img: String)

/** People within 30 m right now; the presence service fills it from the socket. */
val nearbyCards = MutableStateFlow<List<Card>>(emptyList())

class ApiError(val code: Int, message: String) : IOException(message)

/** Runs a server call; returns null when it worked, otherwise the message to show (the server's, or "offline"). */
suspend fun tryServer(call: suspend () -> Unit): String? = try {
    call()
    null
} catch (e: ApiError) {
    e.message ?: OFFLINE
} catch (e: IOException) {
    OFFLINE
}

fun Context.granted(permission: String) = checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED

object Api {
    private val http = OkHttpClient.Builder().pingInterval(25, TimeUnit.SECONDS).build()
    private val jsonType = "application/json".toMediaType()

    private fun prefs(context: Context) = context.getSharedPreferences("session", Context.MODE_PRIVATE)

    fun token(context: Context): String? = prefs(context).getString("token", null)

    /** The server no longer knows this token (account deleted, database reset): sign up again next time. */
    fun forget(context: Context) = prefs(context).edit().remove("token").apply()

    // shortcut: an anonymous account per phone until sign-in (Apple, Google, email code) exists.
    private suspend fun session(context: Context): String = token(context) ?: withContext(Dispatchers.IO) {
        val request = Request.Builder().url("${BuildConfig.API_BASE}/api/session").post(ByteArray(0).toRequestBody()).build()
        // A whole room opening Kinjo at once can be turned away for a moment: try again quietly, spread out.
        val reply = run {
            repeat(2) { attempt ->
                try {
                    return@run call(request)
                } catch (e: IOException) {
                    delay(300L * (attempt + 1) + Random.nextLong(500))
                }
            }
            call(request)
        }
        JSONObject(reply).getString("token").also { prefs(context).edit().putString("token", it).commit() }
    }

    /** Uploads the card. Throws ApiError with the server's message when it refuses. */
    suspend fun saveProfile(context: Context, profile: Profile, photoJpeg: ByteArray) {
        val body = JSONObject()
            .put("name", profile.name.trim())
            .put("role", profile.role.trim())
            .put("looking", profile.looking.trim())
            .put("photo", Base64.encodeToString(photoJpeg, Base64.NO_WRAP))
            .put("agreed", true) // the screen only allows saving after the 18+ and terms box is ticked
            .toString()
            .toRequestBody(jsonType)
        suspend fun put() = withContext(Dispatchers.IO) {
            call(Request.Builder().url("${BuildConfig.API_BASE}/api/profile").header("Authorization", "Bearer ${session(context)}").put(body).build())
        }
        try {
            put()
        } catch (e: ApiError) {
            if (e.code != 401) throw e
            forget(context)
            put() // once more with a fresh account
        }
    }

    /** Whether this account is visible; the server is the authority, this is its last answer. */
    fun visible(context: Context) = prefs(context).getBoolean("visible", true)

    fun rememberVisible(context: Context, visible: Boolean) = prefs(context).edit().putBoolean("visible", visible).apply()

    suspend fun setVisible(context: Context, visible: Boolean) {
        val body = JSONObject().put("visible", visible).toString().toRequestBody(jsonType)
        withContext(Dispatchers.IO) { call(authed(context).url("${BuildConfig.API_BASE}/api/visibility").put(body).build()) }
        rememberVisible(context, visible)
    }

    /** Deletes the account on the server, then this phone's copy of it. */
    suspend fun deleteAccount(context: Context) {
        withContext(Dispatchers.IO) { call(authed(context).url("${BuildConfig.API_BASE}/api/account").delete().build()) }
        prefs(context).edit().clear().commit()
    }

    private suspend fun post(context: Context, path: String, body: JSONObject) {
        withContext(Dispatchers.IO) {
            call(authed(context).url("${BuildConfig.API_BASE}$path").post(body.toString().toRequestBody(jsonType)).build())
        }
    }

    /** Sends a health snapshot in the background; `onFail` runs if it couldn't be delivered. */
    fun sendHealth(context: Context, snapshot: JSONObject, onFail: () -> Unit) {
        background.launch {
            runCatching { post(context, "/api/health", snapshot) }.onFailure {
                trail(context, "health not sent: ${it.message}")
                onFail()
            }
        }
    }

    /** Tells the server this phone's push token, so it can be woken. Skipped until the card is uploaded. */
    suspend fun registerDevice(context: Context, pushToken: String) {
        if (token(context) == null) return
        post(context, "/api/device", JSONObject().put("token", pushToken).put("platform", "android"))
    }

    /** Sends the current push token in the background; if offline, the next app start sends it. */
    fun syncPushToken(context: Context) {
        val app = context.applicationContext
        FirebaseMessaging.getInstance().token.addOnSuccessListener { pushToken ->
            background.launch {
                runCatching { registerDevice(app, pushToken) }.onFailure { trail(app, "push token not sent: ${it.message}") }
            }
        }
    }

    private val background = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    private fun authed(context: Context) =
        Request.Builder().header("Authorization", "Bearer ${token(context) ?: throw ApiError(401, "Sign in again.")}")

    fun socket(token: String, listener: WebSocketListener): WebSocket {
        val url = BuildConfig.API_BASE.replaceFirst("http", "ws") + "/ws"
        return http.newWebSocket(Request.Builder().url(url).header("Authorization", "Bearer $token").build(), listener)
    }

    private fun call(request: Request): String = http.newCall(request).execute().use { response ->
        val text = response.body?.string().orEmpty()
        if (!response.isSuccessful) {
            val message = runCatching { JSONObject(text).getString("error") }.getOrDefault("Something went wrong. Try again.")
            throw ApiError(response.code, message)
        }
        text
    }
}
