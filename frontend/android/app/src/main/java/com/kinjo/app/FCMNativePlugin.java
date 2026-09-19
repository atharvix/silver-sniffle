package com.kinjo.app;

import android.Manifest;
import android.content.Context;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.os.Build;
import android.util.Log;

import androidx.core.content.ContextCompat;

import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.google.firebase.messaging.FirebaseMessaging;

@CapacitorPlugin(
    name = "FCMNative",
    permissions = {
        @Permission(
            strings = { Manifest.permission.POST_NOTIFICATIONS },
            alias = "notifications"
        )
    }
)
public class FCMNativePlugin extends Plugin {

    private static final String TAG = "FCMNativePlugin";
    private static final String PREFS_NAME = "kinjo_prefs";

    @PluginMethod
    public void getFCMToken(PluginCall call) {
        // First check cached token in SharedPreferences
        Context context = getContext();
        SharedPreferences prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);
        String cachedToken = prefs.getString(KinjoFirebaseMessagingService.KEY_FCM_TOKEN, null);

        if (cachedToken != null && !cachedToken.isEmpty()) {
            JSObject ret = new JSObject();
            ret.put("token", cachedToken);
            call.resolve(ret);
            return;
        }

        // Otherwise fetch fresh from Firebase SDK
        try {
            if (com.google.firebase.FirebaseApp.getApps(context).isEmpty()) {
                com.google.firebase.FirebaseApp.initializeApp(context);
            }
            FirebaseMessaging.getInstance().getToken()
                .addOnCompleteListener(task -> {
                    if (!task.isSuccessful()) {
                        Exception ex = task.getException();
                        Log.w(TAG, "Fetching FCM registration token failed", ex);
                        call.reject("Failed to get FCM token: " + (ex != null ? ex.getMessage() : "unknown error"));
                        return;
                    }

                    String token = task.getResult();
                    Log.d(TAG, "Fetched fresh FCM token: " + token);

                    // Cache it
                    prefs.edit().putString(KinjoFirebaseMessagingService.KEY_FCM_TOKEN, token).apply();

                    JSObject ret = new JSObject();
                    ret.put("token", token);
                    call.resolve(ret);
                });
        } catch (Throwable t) {
            Log.w(TAG, "Firebase unavailable: " + t.getMessage());
            call.reject("Firebase unavailable: " + t.getMessage());
        }
    }

    @PluginMethod
    public void checkNotificationPermission(PluginCall call) {
        JSObject ret = new JSObject();
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            boolean granted = ContextCompat.checkSelfPermission(
                getContext(),
                Manifest.permission.POST_NOTIFICATIONS
            ) == PackageManager.PERMISSION_GRANTED;
            ret.put("granted", granted);
        } else {
            // Below Android 13, permission is granted at install time
            ret.put("granted", true);
        }
        call.resolve(ret);
    }
}
