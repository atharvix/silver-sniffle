package world.kinjo.presence;

import android.annotation.SuppressLint;
import android.location.Location;
import android.os.Looper;
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
    static final float USABLE_ACC = 50f; // matches the server's maxAccM

    private final FusedLocationProviderClient fused;
    private final Consumer<Location> onFix;
    private final LocationCallback cb = new LocationCallback() {
        @Override
        public void onLocationResult(LocationResult r) {
            Location l = r.getLastLocation();
            if (l != null) fix(l);
        }
    };
    private boolean fg, on;
    private long interval;

    Gps(PresenceService s, Consumer<Location> onFix) {
        this.fused = LocationServices.getFusedLocationProviderClient(s);
        this.onFix = onFix;
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
    }

    private void now() {
        CurrentLocationRequest req = new CurrentLocationRequest.Builder()
            .setPriority(Priority.PRIORITY_HIGH_ACCURACY)
            .setMaxUpdateAgeMillis(0)
            .setDurationMillis(30_000)
            .build();
        fused.getCurrentLocation(req, null).addOnSuccessListener(l -> { if (l != null) fix(l); });
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
        onFix.accept(l);
    }

    static long ageMs(Location l) {
        return Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
    }
}
