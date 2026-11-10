package world.kinjo.presence;

import android.Manifest;
import android.annotation.SuppressLint;
import android.content.Context;
import android.content.pm.PackageManager;
import android.hardware.Sensor;
import android.hardware.SensorManager;
import android.hardware.TriggerEvent;
import android.hardware.TriggerEventListener;
import android.location.Location;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import androidx.annotation.NonNull;
import androidx.core.content.ContextCompat;
import com.google.android.gms.location.CurrentLocationRequest;
import com.google.android.gms.location.FusedLocationProviderClient;
import com.google.android.gms.location.LocationCallback;
import com.google.android.gms.location.LocationRequest;
import com.google.android.gms.location.LocationResult;
import com.google.android.gms.location.LocationServices;
import com.google.android.gms.location.Priority;

/**
 * Location, the main signal (v3, docs/proximity.md). Runs while the service holds the location
 * type, which only a start from the open app gives it: then it keeps going locked and swiped
 * ("while in use" covers a foreground service started from the app). How hard it works follows
 * the server's answer to each report (mode, next):
 *  - with / near, or the app on screen: precise, a fix every 5 m moved (at most 1 a second);
 *  - idle: Wi-Fi/cell only (no GPS chip), a fix every 100 m;
 *  - still (fixes stayed within STILL_M for STILL_MS): one precise fix, then no updates at all.
 *    The same fix goes out with still:true every `next` s; the significant-motion sensor (no
 *    permission) brings updates back the moment the phone moves.
 * Nothing goes out more often than once a second; nothing waits longer than `next`.
 */
@SuppressLint("MissingPermission") // located() checked before every request
final class Gps {
    static final long STILL_MS = 120_000, MIN_SEND_MS = 1_000;
    static final float STILL_M = 25;

    interface Reporter {
        void report(Location l, boolean still);
    }

    private final Context ctx;
    private final FusedLocationProviderClient fused;
    private final Reporter out;
    private final Handler h = new Handler(Looper.getMainLooper());
    private final SensorManager sensors;
    private final Sensor motion; // null on phones without one: they never go still
    private boolean on, open, still, settling, registered, registeredPrecise;
    private String mode = "near"; // until the server says otherwise
    private long nextMs = 15_000, sentAt, anchorAt;
    private Location last, anchor;

    private final LocationCallback updates = new LocationCallback() {
        @Override
        public void onLocationResult(@NonNull LocationResult r) {
            Location l = r.getLastLocation();
            if (l != null) fixed(l);
        }
    };
    private final TriggerEventListener moved = new TriggerEventListener() {
        @Override
        public void onTrigger(TriggerEvent e) {
            h.post(() -> { if (on && still) wake(); });
        }
    };
    private final Runnable beat = () -> { if (on && last != null) send(last); };

    Gps(Context ctx, Reporter out) {
        this.ctx = ctx;
        this.fused = LocationServices.getFusedLocationProviderClient(ctx);
        this.out = out;
        this.sensors = (SensorManager) ctx.getSystemService(Context.SENSOR_SERVICE);
        this.motion = sensors == null ? null : sensors.getDefaultSensor(Sensor.TYPE_SIGNIFICANT_MOTION);
    }

    static boolean located(Context c) {
        return ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
            || ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED;
    }

    boolean running() {
        return on;
    }

    void start() {
        if (on) return;
        on = true;
        still = false;
        anchor = null;
        register();
        now();
    }

    void stop() {
        on = false;
        unregister();
        disarm();
        h.removeCallbacksAndMessages(null);
    }

    /** The app came to the screen (a fresh precise fix now; precise while open) or left it. */
    void foreground(boolean v) {
        open = v;
        if (!on) return;
        if (v && still) wake();
        else register();
        if (v) now();
    }

    /** The server's answer to a report. */
    void server(String m, int nextS) {
        if (m != null) mode = m;
        if (nextS > 0) nextMs = Math.max(MIN_SEND_MS, nextS * 1000L);
        if (!on) return;
        if (!still) register();
        h.removeCallbacks(beat);
        h.postDelayed(beat, nextMs);
    }

    String mode() {
        return mode;
    }

    boolean still() {
        return still;
    }

    private boolean precise() {
        return open || !"idle".equals(mode);
    }

    private void register() {
        if (!on || still || !located(ctx)) return;
        boolean p = precise();
        if (registered && registeredPrecise == p) return;
        LocationRequest req = new LocationRequest.Builder(p ? Priority.PRIORITY_HIGH_ACCURACY : Priority.PRIORITY_BALANCED_POWER_ACCURACY, p ? 1_000 : 60_000)
            .setMinUpdateDistanceMeters(p ? 5 : 100)
            .setMinUpdateIntervalMillis(MIN_SEND_MS)
            .setWaitForAccurateLocation(false)
            .build();
        try {
            fused.requestLocationUpdates(req, updates, Looper.getMainLooper());
            registered = true;
            registeredPrecise = p;
        } catch (SecurityException e) {
            registered = false; // location taken away in settings: the app shows its card next time it opens
        }
    }

    private void unregister() {
        if (!registered) return;
        fused.removeLocationUpdates(updates);
        registered = false;
    }

    /** One precise fix now, whatever the mode. */
    private void now() {
        current(f -> { if (f != null) fixed(f); });
    }

    private void current(java.util.function.Consumer<Location> then) {
        if (!located(ctx)) { then.accept(null); return; }
        CurrentLocationRequest req = new CurrentLocationRequest.Builder()
            .setPriority(Priority.PRIORITY_HIGH_ACCURACY)
            .setMaxUpdateAgeMillis(5_000)
            .setDurationMillis(10_000)
            .build();
        try {
            fused.getCurrentLocation(req, null)
                .addOnSuccessListener(l -> { if (on) then.accept(l); })
                .addOnFailureListener(e -> { if (on) then.accept(null); })
                .addOnCanceledListener(() -> { if (on) then.accept(null); });
        } catch (SecurityException e) {
            then.accept(null);
        }
    }

    private void fixed(Location l) {
        if (!on || still) return;
        last = l;
        long t = SystemClock.elapsedRealtime();
        if (anchor == null || l.distanceTo(anchor) > Math.max(STILL_M, l.getAccuracy())) {
            anchor = l;
            anchorAt = t;
        } else if (t - anchorAt >= STILL_MS && motion != null && !open && !settling) {
            settle();
            return;
        }
        send(l);
    }

    /** Stopped moving: one precise fix (an idle phone's Wi-Fi/cell fix can be 100 m+ off and
     *  would never match the people arriving), then the GPS chip rests. */
    private void settle() {
        settling = true;
        current(f -> {
            settling = false;
            if (!on || still) return;
            if (f != null && (!last.hasAccuracy() || f.getAccuracy() <= last.getAccuracy())) last = f;
            if (!sensors.requestTriggerSensor(moved, motion)) { anchorAt = SystemClock.elapsedRealtime(); return; } // can't wake on motion: stay moving
            still = true;
            unregister();
            send(last);
        });
    }

    /** Moving again: updates back in the current mode, and a fix now. */
    private void wake() {
        still = false;
        anchor = null;
        disarm();
        register();
        now();
    }

    private void disarm() {
        if (motion != null) sensors.cancelTriggerSensor(moved, motion);
    }

    private void send(Location l) {
        long t = SystemClock.elapsedRealtime();
        if (t - sentAt < MIN_SEND_MS) { // the latest goes out when the second is up
            h.removeCallbacks(beat);
            h.postDelayed(beat, MIN_SEND_MS - (t - sentAt));
            return;
        }
        sentAt = t;
        out.report(l, still);
        h.removeCallbacks(beat);
        h.postDelayed(beat, nextMs);
    }

    static long ageMs(Location l) {
        return Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
    }
}
