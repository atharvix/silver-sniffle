package com.kinjo.app

import android.Manifest
import android.annotation.SuppressLint
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import com.google.android.gms.location.ActivityRecognition
import com.google.android.gms.location.ActivityTransition
import com.google.android.gms.location.ActivityTransitionRequest
import com.google.android.gms.location.ActivityTransitionResult
import com.google.android.gms.location.DetectedActivity
import com.google.android.gms.location.Geofence
import com.google.android.gms.location.GeofencingEvent
import com.google.android.gms.location.GeofencingRequest
import com.google.android.gms.location.LocationServices

/**
 * Restarts presence when Android tells us the person started moving or left the spot where they were still.
 * Both are official exceptions that let a closed app start its background service again. Play services keeps
 * these requests while the app is closed; they are cleared by a reboot, after which the boot receiver re-arms them.
 */
class RestartReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val why = when {
            ActivityTransitionResult.hasResult(intent) -> "moving"
            GeofencingEvent.fromIntent(intent)?.let { !it.hasError() && it.geofenceTransition == Geofence.GEOFENCE_TRANSITION_EXIT } == true -> "left spot"
            else -> return
        }
        PresenceService.start(context, why)
    }

    companion object {
        private const val SPOT_RADIUS_M = 150f // Android's geofences are unreliable below ~100 m

        private fun granted(context: Context, permission: String) =
            context.checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED

        // Mutable: Play services adds the event to the intent.
        private fun pending(context: Context, action: String, code: Int) = PendingIntent.getBroadcast(
            context, code, Intent(context, RestartReceiver::class.java).setAction(action),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
        )

        /** Wake us when the person starts walking, running, cycling or driving. Repeating it replaces the request. */
        @SuppressLint("MissingPermission") // checked first
        fun watchMovement(context: Context) {
            if (!granted(context, Manifest.permission.ACTIVITY_RECOGNITION)) return
            val transitions = listOf(DetectedActivity.WALKING, DetectedActivity.RUNNING, DetectedActivity.ON_BICYCLE, DetectedActivity.IN_VEHICLE)
                .map { ActivityTransition.Builder().setActivityType(it).setActivityTransition(ActivityTransition.ACTIVITY_TRANSITION_ENTER).build() }
            ActivityRecognition.getClient(context)
                .requestActivityTransitionUpdates(ActivityTransitionRequest(transitions), pending(context, "moving", 1))
                .addOnFailureListener { trail(context, "watch movement failed: ${it.message}") }
        }

        /** Wake us when the person leaves the spot where they were still. Repeating it moves the spot. */
        @SuppressLint("MissingPermission") // checked first
        fun watchSpot(context: Context, lat: Double, lng: Double) {
            if (!granted(context, Manifest.permission.ACCESS_FINE_LOCATION) ||
                !granted(context, Manifest.permission.ACCESS_BACKGROUND_LOCATION)
            ) return
            val fence = Geofence.Builder()
                .setRequestId("spot")
                .setCircularRegion(lat, lng, SPOT_RADIUS_M)
                .setExpirationDuration(Geofence.NEVER_EXPIRE)
                .setTransitionTypes(Geofence.GEOFENCE_TRANSITION_EXIT)
                .build()
            val request = GeofencingRequest.Builder().setInitialTrigger(0).addGeofence(fence).build()
            LocationServices.getGeofencingClient(context)
                .addGeofences(request, pending(context, "left spot", 2))
                .addOnFailureListener { trail(context, "watch spot failed: ${it.message}") }
        }
    }
}
