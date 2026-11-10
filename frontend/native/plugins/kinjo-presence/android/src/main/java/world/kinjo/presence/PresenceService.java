package world.kinjo.presence;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.BroadcastReceiver;
import android.content.IntentFilter;
import android.location.LocationManager;
import androidx.core.location.LocationManagerCompat;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.location.Location;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import androidx.core.app.NotificationCompat;
import androidx.core.app.ServiceCompat;
import androidx.core.content.ContextCompat;
import java.util.Locale;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * Keeps the signed-in user findable by Kinjo phones within ~30 m whether Kinjo is open, in the
 * background, closed from recent apps, or the phone just rebooted. Location (Gps) is the main
 * signal: it runs once the open app has started us (Android grants the location type only
 * then), and keeps running locked and swiped. Bluetooth (Ble), if allowed, runs too, for older
 * app builds; it is all a background restart (boot, Check, push) can bring back until the app
 * is opened again. The server decides who sees whom (docs/proximity.md).
 * Brought back by START_STICKY, BootReceiver, Check (every 15 min) and WakeService (push).
 */
public class PresenceService extends Service {
    static final String PREFS = "kinjo_presence", CHANNEL = "presence_quiet";
    private static final int NOTIFICATION_ID = 30;
    private static volatile PresenceService live;
    private static volatile boolean appVisible;

    private Net net;
    private Live socket; // the service's socket: fixes up, mode down (HTTP when it's down)
    private Ble ble; // null without Bluetooth permission
    private Gps gps;
    private final Handler h = new Handler(Looper.getMainLooper());

    static void save(Context c, String api, String token) {
        c.getSharedPreferences(PREFS, MODE_PRIVATE).edit().putString("api", api).putString("token", token).apply();
    }

    static boolean signedIn(Context c) {
        return c.getSharedPreferences(PREFS, MODE_PRIVATE).getString("token", null) != null;
    }

    static boolean alive() {
        return live != null;
    }

    static void start(Context c) {
        try {
            ContextCompat.startForegroundService(c, new Intent(c, PresenceService.class));
        } catch (RuntimeException e) {
            // Android refused this start (background limits); the next app open, check or high-priority push tries again
        }
    }

    /** From the background (boot, periodic check, wake push): only when it can actually work
     *  (location can't start from the background, so that means Bluetooth). */
    static void revive(Context c) {
        if (live == null && signedIn(c) && Ble.allowed(c)) start(c);
    }

    /** A wake push or the server's "scan": a running service scans at full speed now; otherwise start it if allowed. */
    static void poke(Context c, boolean mayStart) {
        PresenceService s = live;
        if (s != null) s.h.post(() -> { if (live == s && s.ble != null) s.ble.burst(); });
        else if (mayStart) revive(c);
    }

    /** The app came to the screen or left it. */
    static void foreground(boolean on) {
        appVisible = on;
        PresenceService s = live;
        if (s == null) return;
        if (on && !s.gps.running() && Gps.located(s)) start(s); // started from the background: take the location type now
        s.h.post(() -> {
            if (live != s) return;
            if (s.ble != null) s.ble.foreground(on);
            s.gps.foreground(on);
        });
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        boolean bt = Ble.allowed(this);
        // the location type only from the open app (a background start with it throws on Android 14+);
        // once held it stays until we stop
        boolean loc = Gps.located(this) && (appVisible || (live == this && gps.running()));
        try { // first, always: a service started with startForegroundService that stops before this crashes the app
            int type = 0;
            if (Build.VERSION.SDK_INT >= 29) {
                if (bt) type |= ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE;
                if (loc) type |= ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION;
            }
            ServiceCompat.startForeground(this, NOTIFICATION_ID, notification(), type);
        } catch (RuntimeException e) {
            stopSelf(); // Android 14+: a permission was taken away, or a background start was refused
            return START_NOT_STICKY;
        }
        if (!signedIn(this) || (!bt && !loc)) {
            stopSelf();
            return START_NOT_STICKY;
        }
        if (live == null) {
            live = this;
            Status.running = true;
            net = new Net(this, this::signedOut);
            gps = new Gps(this, this::report);
            socket = new Live(this, (m, next, sleep) -> { Status.mode = m; gps.server(m, next, sleep); }, this::hiddenElsewhere, this::signedOut, gps::resend);
            registerReceiver(locationSwitch, new IntentFilter(LocationManager.PROVIDERS_CHANGED_ACTION));
            Notify.locationOff(this, !locationOn());
            Check.arm(this);
        }
        if (bt && ble == null) { // allowed now (maybe granted while we ran)
            ble = new Ble(this, net);
            ble.start(appVisible);
        }
        if (loc && !gps.running()) {
            gps.start();
            Notify.cancel(this, Notify.RESTART); // visible again
        }
        gps.foreground(appVisible);
        if (gps.running()) socket.connect();
        return START_STICKY; // killed for memory: Android restarts us (Bluetooth only, until the app opens)
    }

    private void report(Location l, boolean still) {
        Status.fixAt = System.currentTimeMillis() - Gps.ageMs(l);
        Status.fixAcc = l.hasAccuracy() ? l.getAccuracy() : 0;
        Status.still = still;
        PresencePlugin.emit("fix", Status.json());
        int acc = l.hasAccuracy() ? Math.max(1, Math.round(l.getAccuracy())) : 0; // 0 = unknown: the server counts it as vague
        String wifi = Wifi.ids(this);
        String motion = !still && l.hasSpeed() && l.hasBearing() // for the server's crossing prediction
            ? String.format(Locale.US, ",\"spd\":%.1f,\"hdg\":%.0f", l.getSpeed(), l.getBearing()) : "";
        java.util.function.Supplier<String> body = () -> String.format(Locale.US,
            "{\"type\":\"pos\",\"lat\":%.6f,\"lng\":%.6f,\"acc\":%d,\"age\":%d,\"still\":%b,\"wifi\":%s%s}",
            l.getLatitude(), l.getLongitude(), acc, Gps.ageMs(l), still, wifi, motion); // age as sent, not as queued
        if (socket.send(body.get())) return; // the server answers "mode" on the socket
        socket.connect();
        net.post("/api/presence", body,
            resp -> { // {mode, next}: how hard to work from now on
                try {
                    JSONObject o = new JSONObject(resp);
                    Status.mode = o.optString("mode", "near");
                    gps.server(Status.mode, o.optInt("next", 15), o.optInt("sleep", 0));
                } catch (JSONException e) {
                    // an older server answers {ok:true}: keep the current mode
                }
            });
    }

    /** Location switched on or off in quick settings: say so while it's off. */
    private final BroadcastReceiver locationSwitch = new BroadcastReceiver() {
        @Override
        public void onReceive(Context c, Intent i) {
            Notify.locationOff(c, !locationOn());
        }
    };

    private boolean locationOn() {
        LocationManager lm = (LocationManager) getSystemService(Context.LOCATION_SERVICE);
        return lm == null || LocationManagerCompat.isLocationEnabled(lm);
    }

    /** Hidden on another device ("vis"): stop sharing here too; the app restarts us when shown again. */
    private void hiddenElsewhere() {
        Check.disarm(this);
        stopSelf();
    }

    /** 401: the session is gone (signed out elsewhere, account deleted). */
    private void signedOut() {
        save(this, null, null);
        Check.disarm(this);
        stopSelf();
    }

    private Notification notification() {
        if (Build.VERSION.SDK_INT >= 26) {
            // Android requires a notification while the service runs; MIN keeps it as quiet as Android allows
            NotificationManager nm = getSystemService(NotificationManager.class);
            NotificationChannel ch = new NotificationChannel(CHANNEL, "Background status", NotificationManager.IMPORTANCE_MIN);
            ch.setDescription("You can turn this off. Kinjo keeps working and people nearby still see you.");
            ch.setShowBadge(false);
            nm.createNotificationChannel(ch);
            nm.deleteNotificationChannel("presence"); // the old, louder channel
        }
        Intent open = getPackageManager().getLaunchIntentForPackage(getPackageName());
        PendingIntent tap = PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE);
        return new NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_kinjo)
            .setContentTitle("Kinjo")
            .setContentText("Tap to see who’s around you")
            .setPriority(NotificationCompat.PRIORITY_MIN)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setSilent(true)
            .setShowWhen(false)
            .setOngoing(true)
            .setContentIntent(tap)
            .build();
    }

    @Override
    public void onDestroy() {
        if (live == this) {
            live = null;
            gps.stop();
            socket.stop();
            try { unregisterReceiver(locationSwitch); } catch (IllegalArgumentException e) { /* never registered */ }
            Notify.cancel(this, Notify.LOCATION_OFF); // stopped on purpose (hidden, signed out): nothing to fix
            if (ble != null) ble.stop();
            net.shutdown();
        }
        Status.running = false;
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
