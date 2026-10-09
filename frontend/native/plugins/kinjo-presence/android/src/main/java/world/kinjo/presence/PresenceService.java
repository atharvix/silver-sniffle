package world.kinjo.presence;

import android.Manifest;
import android.app.AlarmManager;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.location.Location;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.SystemClock;
import androidx.core.app.NotificationCompat;
import androidx.core.app.ServiceCompat;
import androidx.core.content.ContextCompat;
import java.util.Locale;

/**
 * Keeps the signed-in user's card visible to people within 30 m whether Kinjo is open,
 * in the background, closed from recent apps, or the phone just rebooted. It reports GPS
 * fixes (Gps) and the Kinjo phones it hears over Bluetooth (Ble) straight to the server,
 * which decides who sees whom. Revive and WakeService restart it if the phone kills it.
 */
public class PresenceService extends Service {
    static final String PREFS = "kinjo_presence", CHANNEL = "presence";
    private static final int NOTIFICATION_ID = 30;
    static final long BEAT_MS = 20_000, ALARM_MS = 60_000;
    private static volatile PresenceService live;
    private static volatile boolean appVisible;

    private Net net;
    private Gps gps;
    private Ble ble;
    private final Handler h = new Handler(Looper.getMainLooper());
    private final Runnable beat = new Runnable() { // screen on / awake: check often
        @Override
        public void run() {
            if (live != PresenceService.this) return;
            gps.ensureFresh();
            h.postDelayed(this, BEAT_MS);
        }
    };

    static void save(Context c, String api, String token) {
        c.getSharedPreferences(PREFS, MODE_PRIVATE).edit().putString("api", api).putString("token", token).apply();
    }

    static boolean signedIn(Context c) {
        return c.getSharedPreferences(PREFS, MODE_PRIVATE).getString("token", null) != null;
    }

    static boolean located(Context c) {
        return ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
            || ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED;
    }

    /** From the open app. */
    static void start(Context c) {
        try {
            ContextCompat.startForegroundService(c, new Intent(c, PresenceService.class));
        } catch (RuntimeException e) {
            // Android refused this start; the next app open, geofence, check or wake push tries again
        }
    }

    /** From the background (boot, geofence, periodic check, wake push): only when it can actually work. */
    static void revive(Context c) {
        if (live == null && signedIn(c) && PresencePlugin.hasBackground(c)) start(c);
    }

    /** The app came to the screen or left it: faster updates while someone is looking. */
    static void foreground(boolean on) {
        appVisible = on;
        PresenceService s = live;
        if (s == null) return;
        s.gps.foreground(on);
        s.ble.foreground(on);
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (!signedIn(this) || !located(this)) {
            stopSelf();
            return START_NOT_STICKY;
        }
        try {
            int type = Build.VERSION.SDK_INT >= 29 ? ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION : 0;
            ServiceCompat.startForeground(this, NOTIFICATION_ID, notification(), type);
        } catch (RuntimeException e) {
            stopSelf(); // e.g. started in the background without "Allow all the time"
            return START_NOT_STICKY;
        }
        if (live == null) {
            live = this;
            Status.running = true;
            net = new Net(this, this::signedOut);
            gps = new Gps(this, this::report);
            ble = new Ble(this, net);
            gps.start(appVisible);
            ble.start(appVisible);
            Revive.arm(this);
            h.postDelayed(beat, BEAT_MS);
            alarm(this);
        }
        return START_STICKY; // killed for memory: Android restarts us
    }

    private void report(Location l) {
        Status.fixAt = System.currentTimeMillis() - Gps.ageMs(l);
        Status.fixAcc = l.hasAccuracy() ? l.getAccuracy() : 0;
        PresencePlugin.emit("fix", Status.json());
        int acc = l.hasAccuracy() ? Math.max(1, Math.round(l.getAccuracy())) : 0; // 0 = unknown: the server won't match on it
        net.post("/api/presence", String.format(Locale.US, "{\"lat\":%.6f,\"lng\":%.6f,\"acc\":%d,\"age\":%d}",
            l.getLatitude(), l.getLongitude(), acc, Gps.ageMs(l)));
        Revive.onFix(this, l);
    }

    /**
     * The Handler above stops while the phone sleeps. This alarm still fires then (Android
     * may space it out in deep sleep), wakes the CPU and asks for a fix if none went out
     * recently, so a phone lying still keeps its card on everyone's deck.
     */
    static void alarm(Context c) {
        AlarmManager am = c.getSystemService(AlarmManager.class);
        if (am == null) return;
        PendingIntent pi = PendingIntent.getBroadcast(c, 1, new Intent(c, Beat.class), PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        am.setAndAllowWhileIdle(AlarmManager.ELAPSED_REALTIME_WAKEUP, SystemClock.elapsedRealtime() + ALARM_MS, pi);
    }

    public static class Beat extends BroadcastReceiver {
        @Override
        public void onReceive(Context c, Intent i) {
            PresenceService s = live;
            if (s == null) {
                PresenceService.revive(c);
                return;
            }
            s.gps.ensureFresh();
            alarm(c);
        }
    }

    /** 401: the session is gone (signed out on another device, account deleted). */
    private void signedOut() {
        save(this, null, null);
        Revive.disarm(this);
        stopSelf();
    }

    private Notification notification() {
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager.class)
                .createNotificationChannel(new NotificationChannel(CHANNEL, "Visible nearby", NotificationManager.IMPORTANCE_LOW));
        }
        Intent open = getPackageManager().getLaunchIntentForPackage(getPackageName());
        PendingIntent tap = PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE);
        return new NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.ic_menu_mylocation)
            .setContentTitle("Kinjo")
            .setContentText("You’re visible to people within 30 m")
            .setOngoing(true)
            .setContentIntent(tap)
            .build();
    }

    @Override
    public void onDestroy() {
        if (live == this) {
            live = null;
            h.removeCallbacks(beat);
            gps.stop();
            ble.stop();
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
