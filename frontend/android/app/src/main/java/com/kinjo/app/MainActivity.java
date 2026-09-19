package com.kinjo.app;

import android.Manifest;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Bundle;
import android.util.Log;

import androidx.core.app.ActivityCompat;
import androidx.core.content.ContextCompat;

import com.getcapacitor.BridgeActivity;
import com.google.firebase.FirebaseApp;
import com.google.firebase.messaging.FirebaseMessaging;
import com.shardev.capacitor.googleauth.GoogleAuthPlugin;

public class MainActivity extends BridgeActivity {
    private static final String TAG = "MainActivity";
    private static final int REQUEST_NOTIFICATION_PERMISSION = 1001;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        // Register Capacitor Plugins before super.onCreate
        registerPlugin(GoogleAuthPlugin.class);
        registerPlugin(FCMNativePlugin.class);

        super.onCreate(savedInstanceState);

        // Safe Firebase initialization
        try {
            if (FirebaseApp.getApps(this).isEmpty()) {
                FirebaseApp.initializeApp(this);
            }
        } catch (Throwable t) {
            Log.w(TAG, "FirebaseApp initialization skipped/failed: " + t.getMessage());
        }

        // Initialize Notification Channel
        createNotificationChannel();

        // Request POST_NOTIFICATIONS runtime permission on Android 13+ (API 33+)
        requestNotificationPermissionIfNeeded();

        // Log initial FCM token for debugging / verification
        fetchAndLogFCMToken();
    }

    private void createNotificationChannel() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                String channelId = getString(R.string.default_notification_channel_id);
                CharSequence name = getString(R.string.default_notification_channel_name);
                String description = getString(R.string.default_notification_channel_description);
                int importance = NotificationManager.IMPORTANCE_HIGH;

                NotificationChannel channel = new NotificationChannel(channelId, name, importance);
                channel.setDescription(description);
                channel.enableLights(true);
                channel.enableVibration(true);

                NotificationManager notificationManager = getSystemService(NotificationManager.class);
                if (notificationManager != null) {
                    notificationManager.createNotificationChannel(channel);
                }
            }
        } catch (Throwable t) {
            Log.w(TAG, "createNotificationChannel failed: " + t.getMessage());
        }
    }

    private void requestNotificationPermissionIfNeeded() {
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                    ActivityCompat.requestPermissions(this, new String[]{Manifest.permission.POST_NOTIFICATIONS}, REQUEST_NOTIFICATION_PERMISSION);
                }
            }
        } catch (Throwable t) {
            Log.w(TAG, "requestNotificationPermissionIfNeeded failed: " + t.getMessage());
        }
    }

    private void fetchAndLogFCMToken() {
        try {
            if (FirebaseApp.getApps(this).isEmpty()) {
                FirebaseApp.initializeApp(this);
            }
            FirebaseMessaging.getInstance().getToken()
                .addOnCompleteListener(task -> {
                    if (!task.isSuccessful()) {
                        Log.w(TAG, "Fetching initial FCM token failed", task.getException());
                        return;
                    }
                    String token = task.getResult();
                    Log.i(TAG, "Kinjo FCM Token ready: " + token);
                });
        } catch (Throwable t) {
            Log.w(TAG, "FCM initialization skipped or failed: " + t.getMessage());
        }
    }
}
