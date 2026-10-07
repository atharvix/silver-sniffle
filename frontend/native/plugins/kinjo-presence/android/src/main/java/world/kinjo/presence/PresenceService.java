package world.kinjo.presence;

import android.Manifest;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.location.Location;
import android.os.Build;
import android.os.IBinder;
import android.os.Looper;
import android.os.SystemClock;
import androidx.core.app.NotificationCompat;
import androidx.core.app.ServiceCompat;
import androidx.core.content.ContextCompat;
import com.google.android.gms.location.FusedLocationProviderClient;
import com.google.android.gms.location.LocationCallback;
import com.google.android.gms.location.LocationRequest;
import com.google.android.gms.location.LocationResult;
import com.google.android.gms.location.LocationServices;
import com.google.android.gms.location.Priority;
import java.io.IOException;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Reports the signed-in user's position to POST /api/presence on its own, so their card
 * stays visible nearby with the screen off, the app closed, or after a reboot. The server
 * decides who sees whom; this only keeps the position fresh (the server forgets a position
 * after 2 minutes without a new fix).
 */
public class PresenceService extends Service {
    // Battery vs freshness: a fix every 30 s when still (four chances inside the server's
    // 2-minute window), every 5 s when walking (~7 m apart) so 30 m edges are crossed promptly.
    private static final long STILL_MS = 30_000, MOVING_MS = 5_000;
    private static final float MOVING_MPS = 0.7f;

    private static final String PREFS = "kinjo_presence", CHANNEL = "presence";
    private static final int NOTIFICATION_ID = 30;

    private final ExecutorService net = Executors.newSingleThreadExecutor();
    private final LocationCallback onFix = new LocationCallback() {
        @Override
        public void onLocationResult(LocationResult r) {
            Location l = r.getLastLocation();
            if (l != null) report(l);
        }
    };
    private FusedLocationProviderClient fused;
    private long interval;

    static void save(Context c, String api, String token) {
        c.getSharedPreferences(PREFS, MODE_PRIVATE).edit().putString("api", api).putString("token", token).apply();
    }

    static boolean signedIn(Context c) {
        return c.getSharedPreferences(PREFS, MODE_PRIVATE).getString("token", null) != null;
    }

    static void start(Context c) {
        try {
            ContextCompat.startForegroundService(c, new Intent(c, PresenceService.class));
        } catch (IllegalStateException e) {
            // Android refused a background start; the next app open starts it again.
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        boolean located = ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
            || ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED;
        if (!signedIn(this) || !located) {
            stopSelf();
            return START_NOT_STICKY;
        }
        try {
            int type = Build.VERSION.SDK_INT >= 29 ? ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION : 0;
            ServiceCompat.startForeground(this, NOTIFICATION_ID, notification(), type);
        } catch (RuntimeException e) {
            stopSelf(); // e.g. restarted in the background without "Allow all the time"
            return START_NOT_STICKY;
        }
        if (fused == null) {
            fused = LocationServices.getFusedLocationProviderClient(this);
            request(STILL_MS);
        }
        return START_STICKY; // if Android kills us for memory, it restarts us
    }

    private void request(long ms) {
        interval = ms;
        try {
            fused.requestLocationUpdates(new LocationRequest.Builder(Priority.PRIORITY_HIGH_ACCURACY, ms).build(), onFix, Looper.getMainLooper());
        } catch (SecurityException e) {
            stopSelf(); // permission revoked while running
        }
    }

    private void report(Location l) {
        long want = l.hasSpeed() && l.getSpeed() > MOVING_MPS ? MOVING_MS : STILL_MS;
        if (want != interval) request(want);
        PresencePlugin.emitFix(l.getAccuracy());
        // age: how old the fix already is, so the server dates it by when it was taken.
        long age = Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
        int acc = l.hasAccuracy() ? Math.round(l.getAccuracy()) : 0; // 0 = unknown: the server won't match on it
        String body = String.format(Locale.US, "{\"lat\":%.6f,\"lng\":%.6f,\"acc\":%d,\"age\":%d}", l.getLatitude(), l.getLongitude(), acc, age);
        net.execute(() -> post(body));
    }

    private void post(String body) {
        SharedPreferences p = getSharedPreferences(PREFS, MODE_PRIVATE);
        String api = p.getString("api", null), token = p.getString("token", null);
        if (api == null || token == null) return;
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(api + "/api/presence").openConnection();
            c.setConnectTimeout(10_000);
            c.setReadTimeout(10_000);
            c.setRequestMethod("POST");
            c.setDoOutput(true);
            c.setRequestProperty("Content-Type", "application/json");
            c.setRequestProperty("Authorization", "Bearer " + token);
            try (OutputStream out = c.getOutputStream()) {
                out.write(body.getBytes(StandardCharsets.UTF_8));
            }
            if (c.getResponseCode() == 401) { // session gone (signed out elsewhere): stop for good
                save(this, null, null);
                stopSelf();
            }
        } catch (IOException e) {
            // Offline: the next fix tries again. Meanwhile the server lets this position expire.
        } finally {
            if (c != null) c.disconnect();
        }
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
        if (fused != null) fused.removeLocationUpdates(onFix);
        net.shutdown();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
