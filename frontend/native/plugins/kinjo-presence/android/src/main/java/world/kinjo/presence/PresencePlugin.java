package world.kinjo.presence;

import android.Manifest;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import androidx.core.content.ContextCompat;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.getcapacitor.annotation.PermissionCallback;

/** The app's handle on {@link PresenceService}: start/stop it and walk through permissions. */
@CapacitorPlugin(
    name = "KinjoPresence",
    permissions = {
        @Permission(alias = "location", strings = { Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION }),
        @Permission(alias = "background", strings = { Manifest.permission.ACCESS_BACKGROUND_LOCATION })
    }
)
public class PresencePlugin extends Plugin {
    private static volatile PresencePlugin live; // set while the app is open, for "fix" events

    @Override
    public void load() {
        live = this;
    }

    @Override
    protected void handleOnDestroy() {
        if (live == this) live = null;
    }

    /** Remember the session and run the service. It keeps running until stop() or a 401. */
    @PluginMethod
    public void start(PluginCall call) {
        String api = call.getString("api"), token = call.getString("token");
        if (api == null || token == null || token.isEmpty()) {
            call.reject("api and token are required");
            return;
        }
        PresenceService.save(getContext(), api, token);
        PresenceService.start(getContext());
        call.resolve();
    }

    /** Forget the session and stop sharing (hidden, signed out, account deleted). */
    @PluginMethod
    public void stop(PluginCall call) {
        PresenceService.save(getContext(), null, null);
        getContext().stopService(new Intent(getContext(), PresenceService.class));
        call.resolve();
    }

    /** {granted}: may Kinjo use location with the app closed? ask:true opens Android's "Allow all the time" choice. */
    @PluginMethod
    public void background(PluginCall call) {
        if (hasBackground(getContext()) || !call.getBoolean("ask", false)) resolveBackground(call);
        else requestPermissionForAlias("background", call, "backgroundDone");
    }

    @PermissionCallback
    private void backgroundDone(PluginCall call) {
        resolveBackground(call);
    }

    private void resolveBackground(PluginCall call) {
        JSObject out = new JSObject();
        out.put("granted", hasBackground(getContext()));
        call.resolve(out);
    }

    /** Kinjo's page in Android settings (Battery, Permissions, Autostart live there). */
    @PluginMethod
    public void openSettings(PluginCall call) {
        Intent i = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", getContext().getPackageName(), null));
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        getContext().startActivity(i);
        call.resolve();
    }

    /** Android 9 and older grant background access along with foreground location. */
    static boolean hasBackground(Context c) {
        String p = Build.VERSION.SDK_INT >= 29 ? Manifest.permission.ACCESS_BACKGROUND_LOCATION : Manifest.permission.ACCESS_FINE_LOCATION;
        return ContextCompat.checkSelfPermission(c, p) == PackageManager.PERMISSION_GRANTED;
    }

    /** Tell the open app a fix arrived (drives "Location found"); a no-op when the app is closed. */
    static void emitFix(float accuracy) {
        PresencePlugin p = live;
        if (p == null) return;
        JSObject o = new JSObject();
        o.put("acc", accuracy);
        p.notifyListeners("fix", o);
    }
}
