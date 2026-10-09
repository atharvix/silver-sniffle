package world.kinjo.presence;

import android.annotation.SuppressLint;
import android.content.Context;
import android.location.Location;
import android.os.Looper;
import android.os.PowerManager;
import android.os.SystemClock;
import com.google.android.gms.location.CurrentLocationRequest;
import com.google.android.gms.location.FusedLocationProviderClient;
import com.google.android.gms.location.LocationCallback;
import com.google.android.gms.location.LocationRequest;
import com.google.android.gms.location.LocationResult;
import com.google.android.gms.location.LocationServices;
import com.google.android.gms.location.Priority;
import java.util.function.Consumer;

/**
 * GPS, the main proximity signal. A fix goes out the moment one exists: the last known
 * position if it's recent, then a fresh one, then regular updates. The server forgets a
 * position after 2 minutes, so the background interval leaves room for several misses.
 */
@SuppressLint("MissingPermission") // the service checks location permission before starting us
final class Gps {
    static final long FG_MS = 10_000, BG_MS = 30_000, MOVING_MS = 5_000;
    static final float MOVING_MPS = 0.7f; // walking pace: ~7 m between fixes at MOVING_MS
    static final long RECENT_MS = 60_000;
    static final float USABLE_ACC = 30f; // matches the server's maxAccM
    static final long STALE_MS = 40_000; // no fix sent this long: ask for one (the server forgets after 120 s)

    private final FusedLocationProviderClient fused;
    private final Consumer<Location> onFix;
    private final LocationCallback cb = new LocationCallback() {
        @Override
        public void onLocationResult(LocationResult r) {
            Location l = r.getLastLocation();
            if (l != null) fix(l);
        }
    };
    private final PowerManager.WakeLock wake;
    private boolean fg, on, asking;
    private long interval, sentAt;

    Gps(PresenceService s, Consumer<Location> onFix) {
        this.fused = LocationServices.getFusedLocationProviderClient(s);
        this.onFix = onFix;
        this.wake = ((PowerManager) s.getSystemService(Context.POWER_SERVICE)).newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "kinjo:fix");
        this.wake.setReferenceCounted(false);
    }

    /**
     * A phone lying still with the screen off can get no new fix for minutes (the fused
     * provider has nothing new to say), and the server would drop the card. The service
     * calls this on a timer: if nothing went out for STALE_MS, ask for a fix now.
     */
    void ensureFresh() {
        if (on && !asking && SystemClock.elapsedRealtime() - sentAt > STALE_MS) now();
    }

    void start(boolean foreground) {
        fg = foreground;
        on = true;
        try {
            fused.getLastLocation().addOnSuccessListener(l -> {
                if (l != null && ageMs(l) < RECENT_MS && l.hasAccuracy() && l.getAccuracy() <= USABLE_ACC) fix(l);
            });
            now();
            request(base());
        } catch (SecurityException e) {
            on = false; // permission revoked: the service notices on its next start
        }
    }

    /** The app came to the screen (faster updates and a fix right now) or left it. */
    void foreground(boolean v) {
        if (!on || fg == v) return;
        fg = v;
        request(base());
        if (v) now();
    }

    void stop() {
        on = false;
        fused.removeLocationUpdates(cb);
        doneAsking();
    }

    /** One fix right now; keeps the CPU awake (30 s at most) until it's in. Falls back to wifi/cell if GPS can't. */
    private void now() {
        if (asking) return;
        asking = true;
        wake.acquire(32_000);
        current(Priority.PRIORITY_HIGH_ACCURACY, () -> current(Priority.PRIORITY_BALANCED_POWER_ACCURACY, this::doneAsking));
    }

    private void current(int priority, Runnable onNone) {
        CurrentLocationRequest req = new CurrentLocationRequest.Builder()
            .setPriority(priority)
            .setMaxUpdateAgeMillis(0)
            .setDurationMillis(15_000)
            .build();
        try {
            fused.getCurrentLocation(req, null)
                .addOnSuccessListener(l -> { if (l != null) { fix(l); doneAsking(); } else onNone.run(); })
                .addOnFailureListener(e -> onNone.run());
        } catch (SecurityException e) {
            doneAsking();
        }
    }

    private void doneAsking() {
        asking = false;
        if (wake.isHeld()) wake.release();
    }

    private long base() {
        return fg ? FG_MS : BG_MS;
    }

    private void request(long ms) {
        if (!on) return;
        interval = ms;
        try { // same callback: replaces the previous request
            fused.requestLocationUpdates(
                new LocationRequest.Builder(Priority.PRIORITY_HIGH_ACCURACY, ms).setMinUpdateIntervalMillis(Math.min(ms, MOVING_MS)).build(),
                cb, Looper.getMainLooper());
        } catch (SecurityException e) {
            on = false;
        }
    }

    private void fix(Location l) {
        if (!on) return;
        long want = l.hasSpeed() && l.getSpeed() > MOVING_MPS ? MOVING_MS : base();
        if (want != interval) request(want);
        sentAt = SystemClock.elapsedRealtime();
        onFix.accept(l);
    }

    static long ageMs(Location l) {
        return Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
    }
}
