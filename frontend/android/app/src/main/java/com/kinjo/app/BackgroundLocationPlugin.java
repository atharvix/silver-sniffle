package com.kinjo.app;

import android.content.Intent;
import android.util.Log;

import androidx.core.content.ContextCompat;

import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/**
 * Starts and stops the background presence service. The web layer hands over
 * the API base and the session token, so the service can publish location
 * without needing access to the WebView's storage.
 */
@CapacitorPlugin(name = "BackgroundLocation")
public class BackgroundLocationPlugin extends Plugin {

    private static final String TAG = "BackgroundLocation";

    @PluginMethod
    public void start(PluginCall call) {
        String apiBase = call.getString("apiBase", "");
        String token = call.getString("token", "");

        if (apiBase == null || apiBase.isEmpty() || token == null || token.isEmpty()) {
            call.reject("apiBase and token are required");
            return;
        }

        try {
            Intent intent = new Intent(getContext(), KinjoLocationService.class);
            intent.setAction(KinjoLocationService.ACTION_START);
            intent.putExtra(KinjoLocationService.EXTRA_API_BASE, apiBase);
            intent.putExtra(KinjoLocationService.EXTRA_TOKEN, token);

            // Must be a foreground start: the service shows its notification
            // immediately and Android 8+ forbids plain background starts.
            ContextCompat.startForegroundService(getContext(), intent);
            call.resolve();
        } catch (Throwable t) {
            Log.w(TAG, "failed to start presence service: " + t.getMessage());
            call.reject("failed to start presence service: " + t.getMessage());
        }
    }

    @PluginMethod
    public void stop(PluginCall call) {
        try {
            getContext().stopService(new Intent(getContext(), KinjoLocationService.class));
            call.resolve();
        } catch (Throwable t) {
            Log.w(TAG, "failed to stop presence service: " + t.getMessage());
            call.reject("failed to stop presence service: " + t.getMessage());
        }
    }
}
