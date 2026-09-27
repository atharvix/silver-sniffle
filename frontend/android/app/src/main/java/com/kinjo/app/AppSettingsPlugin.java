package com.kinjo.app;

import android.content.Intent;
import android.net.Uri;
import android.provider.Settings;
import android.util.Log;

import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;

/**
 * Opens the OS settings screen for this app.
 *
 * Android and iOS only ever show a runtime permission dialog once: once the user
 * has denied (or the system has auto-denied), the prompt never appears again and
 * every later request returns "denied" without any UI. The app's own settings
 * page is then the only way for the user to flip the switch back, so the
 * onboarding steps surface it as a recovery action.
 */
@CapacitorPlugin(name = "AppSettings")
public class AppSettingsPlugin extends Plugin {

    private static final String TAG = "AppSettings";

    @PluginMethod
    public void open(PluginCall call) {
        try {
            Intent intent = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS);
            intent.setData(Uri.fromParts("package", getContext().getPackageName(), null));
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            getContext().startActivity(intent);
            call.resolve();
        } catch (Throwable t) {
            Log.w(TAG, "failed to open app settings: " + t.getMessage());
            call.reject("failed to open app settings: " + t.getMessage());
        }
    }
}
