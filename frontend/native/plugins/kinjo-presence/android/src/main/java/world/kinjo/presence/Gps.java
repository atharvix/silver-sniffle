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
 * the server's answer to each report (mode, next). The GPS chip is the battery, so it runs
 * only when someone could be within range:
 *  - with (someone in range), or the app on screen: GPS, a fix every 5 m moved (≤ 1/s);
 *  - near (someone within ~150-450 m): GPS, every 5 m (≤ 1 per 2 s), except while the server
 *    says nobody can reach our line for `sleep` s: Wi-Fi/cell until then;
 *  - around (someone within ~1 km): Wi-Fi/cell, every 25 m;
 *  - idle (nobody): Wi-Fi/cell, every 100 m, delivered in batches (the CPU wakes less);
 *  - still (fixes stayed within STILL_M for STILL_MS): one precise fix, then no updates at all.
 *    The same fix goes out with still:true every STILL_BEAT_MS; the significant-motion sensor
 *    (no permission) brings updates back the moment the phone moves.
 * Nothing goes out more often than once a second; a moving phone never waits longer than `next`.
 */
@SuppressLint("MissingPermission") // located() checked before every request
final class Gps {
    static final long STILL_MS = 120_000, STILL_BEAT_MS = 120_000, MIN_SEND_MS = 1_000;
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
    private boolean on, open, still, settling;
    private int registered = -1; // the profile now requested (see profile()), -1 = none
    private String mode = "near"; // until the server says otherwise
    private long nextMs = 15_000, sentAt, anchorAt, restUntil, fixAt;
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
    private final Runnable beat = this::beat;
    private final Runnable restOver = () -> { if (on && !still) { register(); now(); } };

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

    /** The server's answer to a report. sleep: seconds GPS may rest (near only). */
    void server(String m, int nextS, int sleepS) {
        if (m != null) mode = m;
        if (nextS > 0) nextMs = Math.max(MIN_SEND_MS, nextS * 1000L);
        h.removeCallbacks(restOver);
        restUntil = "near".equals(mode) && sleepS > 0 ? SystemClock.elapsedRealtime() + sleepS * 1000L : 0;
        if (restUntil > 0) h.postDelayed(restOver, sleepS * 1000L);
        if (!on) return;
        if (!still) register();
        h.removeCallbacks(beat);
        h.postDelayed(beat, beatMs());
    }

    private long beatMs() {
        return still ? STILL_BEAT_MS : nextMs;
    }

    /** The newest fix again now (the socket just opened: the server may have restarted). */
    void resend() {
        if (on && last != null) {
            sentAt = 0;
            send(last);
        }
    }

    String mode() {
        return mode;
    }

    boolean still() {
        return still;
    }

    /** 0 with/open, 1 near, 2 around, 3 idle. */
    private int profile() {
        if (open || "with".equals(mode)) return 0;
        if ("near".equals(mode)) return SystemClock.elapsedRealtime() < restUntil ? 2 : 1;
        if ("around".equals(mode)) return 2;
        return 3;
    }

    private void register() {
        if (!on || still || !located(ctx)) return;
        int p = profile();
        if (registered == p) return;
        LocationRequest.Builder b;
        switch (p) {
            case 0: b = new LocationRequest.Builder(Priority.PRIORITY_HIGH_ACCURACY, 1_000).setMinUpdateDistanceMeters(5); break;
            case 1: b = new LocationRequest.Builder(Priority.PRIORITY_HIGH_ACCURACY, 2_000).setMinUpdateDistanceMeters(5); break;
            case 2: b = new LocationRequest.Builder(Priority.PRIORITY_BALANCED_POWER_ACCURACY, 30_000).setMinUpdateDistanceMeters(25); break;
            default: b = new LocationRequest.Builder(Priority.PRIORITY_BALANCED_POWER_ACCURACY, 60_000).setMinUpdateDistanceMeters(100)
                .setMaxUpdateDelayMillis(120_000); // batched: nobody to match, so no hurry
        }
        try {
            fused.requestLocationUpdates(b.setMinUpdateIntervalMillis(MIN_SEND_MS).setWaitForAccurateLocation(false).build(), updates, Looper.getMainLooper());
            registered = p;
        } catch (SecurityException e) {
            registered = -1; // location taken away in settings: the app shows its card next time it opens
        }
    }

    private void unregister() {
        if (registered < 0) return;
        fused.removeLocationUpdates(updates);
        registered = -1;
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
        fixAt = SystemClock.elapsedRealtime();
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
        h.postDelayed(beat, beatMs());
    }

    /** Nothing sent for `next` (or STILL_BEAT_MS while still): the latest again. */
    private void beat() {
        if (!on || last == null) return;
        // no new fix for a while (deep indoors) and no motion: still, not gone
        if (!still && !settling && motion != null && SystemClock.elapsedRealtime() - fixAt >= STILL_MS) { settle(); return; }
        send(last);
    }

    static long ageMs(Location l) {
        return Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
    }
}
