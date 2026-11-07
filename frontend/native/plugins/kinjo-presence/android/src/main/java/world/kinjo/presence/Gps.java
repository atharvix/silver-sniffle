package world.kinjo.presence;

import android.Manifest;
import android.annotation.SuppressLint;
import android.content.Context;
import android.content.pm.PackageManager;
import android.location.Location;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import androidx.core.content.ContextCompat;
import com.google.android.gms.location.CurrentLocationRequest;
import com.google.android.gms.location.FusedLocationProviderClient;
import com.google.android.gms.location.LocationServices;
import com.google.android.gms.location.Priority;
import java.util.function.Consumer;

/**
 * One GPS fix every EVERY_MS while the app is on screen, none otherwise (Bluetooth is the main
 * signal; good outdoor fixes only let the server veto Bluetooth heard across a street). Android
 * gives "while using" location to the whole process while an activity is visible, so asking
 * from the service is fine and needs no location service type.
 */
@SuppressLint("MissingPermission") // located() checked before every request
final class Gps {
    static final long EVERY_MS = 10_000;

    private final Context ctx;
    private final FusedLocationProviderClient fused;
    private final Consumer<Location> onFix;
    private final Handler h = new Handler(Looper.getMainLooper());
    private boolean on, asking;
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            if (!on) return;
            now();
            h.postDelayed(this, EVERY_MS);
        }
    };

    Gps(Context ctx, Consumer<Location> onFix) {
        this.ctx = ctx;
        this.fused = LocationServices.getFusedLocationProviderClient(ctx);
        this.onFix = onFix;
    }

    static boolean located(Context c) {
        return ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
            || ContextCompat.checkSelfPermission(c, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED;
    }

    /** The app came to the screen (fixes start now) or left it (they stop). */
    void foreground(boolean v) {
        if (on == v) return;
        on = v;
        h.removeCallbacks(tick);
        if (v) h.post(tick);
    }

    private void now() {
        if (asking || !located(ctx)) return;
        asking = true;
        CurrentLocationRequest req = new CurrentLocationRequest.Builder()
            .setPriority(Priority.PRIORITY_HIGH_ACCURACY)
            .setMaxUpdateAgeMillis(5_000)
            .setDurationMillis(8_000)
            .build();
        try {
            fused.getCurrentLocation(req, null)
                .addOnSuccessListener(l -> { asking = false; if (l != null && on) onFix.accept(l); })
                .addOnFailureListener(e -> asking = false)
                .addOnCanceledListener(() -> asking = false);
        } catch (SecurityException e) {
            asking = false;
        }
    }

    static long ageMs(Location l) {
        return Math.max(0, (SystemClock.elapsedRealtimeNanos() - l.getElapsedRealtimeNanos()) / 1_000_000);
    }
}
