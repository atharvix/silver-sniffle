package com.kinjo.app;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.util.Log;

import androidx.annotation.NonNull;
import androidx.annotation.Nullable;
import androidx.core.app.NotificationCompat;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/**
 * Keeps the device's location reaching the backend while the app is in the
 * background or has been swiped away.
 *
 * The WebView cannot be relied on for that: Android pauses it when the app is
 * backgrounded and throttles its timers, so presence silently stopped. A
 * foreground service holds the process alive and publishes location natively,
 * independently of JavaScript.
 *
 * ponytail: sends straight to /api/profiles/location over HttpURLConnection
 * rather than pulling in a dependency for one POST. Swap in OkHttp if this ever
 * needs retries, backoff or a request queue.
 */
public class KinjoLocationService extends Service {

    private static final String TAG = "KinjoLocationService";

    public static final String ACTION_START = "com.kinjo.app.action.START_LOCATION";
    public static final String ACTION_STOP = "com.kinjo.app.action.STOP_LOCATION";
    public static final String EXTRA_API_BASE = "api_base";
    public static final String EXTRA_TOKEN = "token";

    private static final String CHANNEL_ID = "kinjo_presence";
    private static final int NOTIFICATION_ID = 4711;

    /** Location updates are requested at this cadence and distance. */
    private static final long UPDATE_INTERVAL_MS = 20_000L;
    private static final float UPDATE_MIN_DISTANCE_M = 10f;

    /** Skip a publish when nothing moved and the last one is still recent. */
    private static final long MAX_IDLE_MS = 60_000L;

    private final ExecutorService executor = Executors.newSingleThreadExecutor();

    private LocationManager locationManager;
    private LocationListener locationListener;

    private String apiBase;
    private String token;
    private Location lastPublished;
    private long lastPublishedAt;

    @Override
    public int onStartCommand(@Nullable Intent intent, int flags, int startId) {
        if (intent != null) {
            if (ACTION_STOP.equals(intent.getAction())) {
                stopSelf();
                return START_NOT_STICKY;
            }
            String base = intent.getStringExtra(EXTRA_API_BASE);
            String incomingToken = intent.getStringExtra(EXTRA_TOKEN);
            if (base != null && !base.isEmpty()) {
                apiBase = base;
            }
            if (incomingToken != null && !incomingToken.isEmpty()) {
                token = incomingToken;
            }
        }

        startInForeground();
        startLocationUpdates();

        // Restart with the last intent if Android kills the process under memory
        // pressure, so presence resumes without the user reopening the app.
        return START_REDELIVER_INTENT;
    }

    private void startInForeground() {
        try {
            Notification notification = buildNotification();

            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_LOCATION);
            } else {
                startForeground(NOTIFICATION_ID, notification);
            }
        } catch (Throwable t) {
            Log.w(TAG, "startForeground failed: " + t.getMessage());
        }
    }

    private Notification buildNotification() {
        createChannelIfNeeded();

        Intent openApp = new Intent(this, MainActivity.class);
        openApp.setFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP);

        int pendingIntentFlags = PendingIntent.FLAG_UPDATE_CURRENT;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            pendingIntentFlags |= PendingIntent.FLAG_IMMUTABLE;
        }
        PendingIntent contentIntent = PendingIntent.getActivity(this, 0, openApp, pendingIntentFlags);

        return new NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.location_service_title))
            .setContentText(getString(R.string.location_service_text))
            // Monochrome white-on-transparent mark: a full-colour launcher icon
            // renders as an unreadable white blob in the status bar.
            .setSmallIcon(R.mipmap.ic_launcher_foreground)
            .setContentIntent(contentIntent)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build();
    }

    private void createChannelIfNeeded() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return;
        }
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (manager == null || manager.getNotificationChannel(CHANNEL_ID) != null) {
            return;
        }

        NotificationChannel channel = new NotificationChannel(
            CHANNEL_ID,
            getString(R.string.location_channel_name),
            NotificationManager.IMPORTANCE_LOW
        );
        channel.setDescription(getString(R.string.location_channel_description));
        channel.setShowBadge(false);
        manager.createNotificationChannel(channel);
    }

    private void startLocationUpdates() {
        if (locationManager != null) {
            return;
        }

        locationManager = (LocationManager) getSystemService(LOCATION_SERVICE);
        if (locationManager == null) {
            return;
        }

        locationListener = new LocationListener() {
            @Override
            public void onLocationChanged(@NonNull Location location) {
                publish(location);
            }

            // Kept for minSdk 24, where these are still abstract on the
            // framework interface.
            @Override
            public void onStatusChanged(String provider, int status, Bundle extras) { }

            @Override
            public void onProviderEnabled(@NonNull String provider) { }

            @Override
            public void onProviderDisabled(@NonNull String provider) { }
        };

        for (String provider : new String[]{LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER}) {
            try {
                if (!locationManager.isProviderEnabled(provider)) {
                    continue;
                }
                locationManager.requestLocationUpdates(
                    provider, UPDATE_INTERVAL_MS, UPDATE_MIN_DISTANCE_M, locationListener);
            } catch (Throwable t) {
                Log.w(TAG, "location updates unavailable for " + provider + ": " + t.getMessage());
            }
        }

        // Seed an immediate fix so presence is correct right after a restart.
        try {
            Location last = locationManager.getLastKnownLocation(LocationManager.GPS_PROVIDER);
            if (last == null) {
                last = locationManager.getLastKnownLocation(LocationManager.NETWORK_PROVIDER);
            }
            if (last != null) {
                publish(last);
            }
        } catch (Throwable t) {
            Log.w(TAG, "no last known location: " + t.getMessage());
        }
    }

    /** Publishes only when the position actually moved, or the last one aged out. */
    private void publish(Location location) {
        long now = System.currentTimeMillis();
        if (lastPublished != null
            && location.distanceTo(lastPublished) < UPDATE_MIN_DISTANCE_M
            && now - lastPublishedAt < MAX_IDLE_MS) {
            return;
        }

        lastPublished = location;
        lastPublishedAt = now;

        final String base = apiBase;
        final String authToken = token;
        if (base == null || base.isEmpty() || authToken == null || authToken.isEmpty()) {
            return;
        }

        final double latitude = location.getLatitude();
        final double longitude = location.getLongitude();
        executor.execute(() -> postLocation(base, authToken, latitude, longitude));
    }

    private void postLocation(String base, String authToken, double latitude, double longitude) {
        HttpURLConnection connection = null;
        try {
            URL url = new URL(base.replaceAll("/$", "") + "/profiles/location");
            connection = (HttpURLConnection) url.openConnection();
            connection.setRequestMethod("POST");
            connection.setConnectTimeout(15000);
            connection.setReadTimeout(15000);
            connection.setDoOutput(true);
            connection.setRequestProperty("Content-Type", "application/json");
            connection.setRequestProperty("Authorization", "Bearer " + authToken);
            connection.setRequestProperty("ngrok-skip-browser-warning", "true");
            connection.setRequestProperty("bypass-tunnel-reminder", "true");

            String body = "{\"latitude\":" + latitude + ",\"longitude\":" + longitude + "}";
            try (OutputStream out = connection.getOutputStream()) {
                out.write(body.getBytes(StandardCharsets.UTF_8));
            }

            int status = connection.getResponseCode();
            if (status < 200 || status >= 300) {
                Log.w(TAG, "location publish rejected with status " + status);
            }
        } catch (Throwable t) {
            Log.w(TAG, "location publish failed: " + t.getMessage());
        } finally {
            if (connection != null) {
                connection.disconnect();
            }
        }
    }

    @Override
    public void onDestroy() {
        try {
            if (locationManager != null && locationListener != null) {
                locationManager.removeUpdates(locationListener);
            }
        } catch (Throwable t) {
            Log.w(TAG, "removeUpdates failed: " + t.getMessage());
        }
        executor.shutdown();
        super.onDestroy();
    }

    @Nullable
    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
