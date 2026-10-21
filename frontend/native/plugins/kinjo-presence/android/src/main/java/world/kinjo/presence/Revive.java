package world.kinjo.presence;

import android.annotation.SuppressLint;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.location.Location;
import androidx.annotation.NonNull;
import androidx.work.ExistingPeriodicWorkPolicy;
import androidx.work.PeriodicWorkRequest;
import androidx.work.WorkManager;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import com.google.android.gms.location.Geofence;
import com.google.android.gms.location.GeofencingRequest;
import com.google.android.gms.location.LocationServices;
import java.util.Collections;
import java.util.concurrent.TimeUnit;

/**
 * Brings the service back after a phone's battery manager kills it (some Xiaomi, Samsung,
 * Oppo builds do, even for foreground services). Two independent ways, both allowed to
 * start a location service from the background when Kinjo has "Allow all the time":
 * leaving a 150 m geofence around the last fix, and a check every 15 minutes.
 * (The server's wake push, WakeService, is the third.)
 */
final class Revive {
    static final float RADIUS_M = 150f;
    private static final String WORK = "kinjo-revive", FENCE = "kinjo-here";
    private static Location fenced; // centre of the current geofence

    /** Called with each fix: re-centre the geofence once we've moved well inside it. */
    @SuppressLint("MissingPermission")
    static void onFix(Context c, Location l) {
        if (fenced != null && fenced.distanceTo(l) < RADIUS_M / 2) return;
        if (!PresencePlugin.hasBackground(c)) return; // background geofences need "Allow all the time"
        Geofence g = new Geofence.Builder()
            .setRequestId(FENCE)
            .setCircularRegion(l.getLatitude(), l.getLongitude(), RADIUS_M)
            .setExpirationDuration(Geofence.NEVER_EXPIRE)
            .setTransitionTypes(Geofence.GEOFENCE_TRANSITION_EXIT)
            .build();
        GeofencingRequest req = new GeofencingRequest.Builder().setInitialTrigger(0).addGeofence(g).build();
        try {
            LocationServices.getGeofencingClient(c).addGeofences(req, fenceIntent(c)).addOnSuccessListener(v -> fenced = l);
        } catch (SecurityException e) {
            // permission changed: the next fix tries again
        }
    }

    static void arm(Context c) {
        PeriodicWorkRequest w = new PeriodicWorkRequest.Builder(Check.class, 15, TimeUnit.MINUTES).build();
        WorkManager.getInstance(c).enqueueUniquePeriodicWork(WORK, ExistingPeriodicWorkPolicy.KEEP, w);
    }

    /** Signed out or hidden: no more revivals. */
    static void disarm(Context c) {
        WorkManager.getInstance(c).cancelUniqueWork(WORK);
        LocationServices.getGeofencingClient(c).removeGeofences(Collections.singletonList(FENCE));
        fenced = null;
    }

    private static PendingIntent fenceIntent(Context c) {
        Intent i = new Intent(c, Wake.class);
        // geofencing fills in the event, so the intent must be mutable
        return PendingIntent.getBroadcast(c, 0, i, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_MUTABLE);
    }

    /** Geofence exit: we moved and may have been killed meanwhile. */
    public static class Wake extends BroadcastReceiver {
        @Override
        public void onReceive(Context c, Intent i) {
            PresenceService.revive(c);
        }
    }

    /** Every 15 minutes: start the service if it should be running and isn't. */
    public static class Check extends Worker {
        public Check(@NonNull Context c, @NonNull WorkerParameters p) {
            super(c, p);
        }

        @NonNull
        @Override
        public Result doWork() {
            if (!Status.running) PresenceService.revive(getApplicationContext());
            return Result.success();
        }
    }
}
