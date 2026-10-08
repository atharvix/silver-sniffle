package world.kinjo.presence;

import android.Manifest;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.PowerManager;
import android.provider.Settings;
import androidx.core.content.ContextCompat;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.getcapacitor.annotation.PermissionCallback;

/** The app's handle on {@link PresenceService}: start/stop it, walk through permissions, read its status. */
@CapacitorPlugin(
    name = "KinjoPresence",
    permissions = {
        @Permission(alias = "location", strings = { Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION }),
        @Permission(alias = "background", strings = { Manifest.permission.ACCESS_BACKGROUND_LOCATION }),
        @Permission(alias = "bluetooth", strings = { Manifest.permission.BLUETOOTH_SCAN, Manifest.permission.BLUETOOTH_ADVERTISE })
    }
)
public class PresencePlugin extends Plugin {
    private static volatile PresencePlugin live; // set while the app is open, for events

    @Override
    public void load() {
        live = this;
    }

    @Override
    protected void handleOnDestroy() {
        if (live == this) live = null;
    }

    @Override
    protected void handleOnResume() {
        PresenceService.foreground(true);
    }

    @Override
    protected void handleOnPause() {
        PresenceService.foreground(false);
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
        PresenceService.foreground(true); // start() is only called from the open app
        PresenceService.start(getContext());
        call.resolve();
    }

    /** Forget the session and stop sharing (hidden, signed out, account deleted). */
    @PluginMethod
    public void stop(PluginCall call) {
        PresenceService.save(getContext(), null, null);
        Revive.disarm(getContext());
        getContext().stopService(new Intent(getContext(), PresenceService.class));
        call.resolve();
    }

    /** {granted}: may Kinjo use location with the app closed? ask:true opens Android's "Allow all the time" choice. */
    @PluginMethod
    public void background(PluginCall call) {
        if (hasBackground(getContext()) || !call.getBoolean("ask", false)) resolveGranted(call, hasBackground(getContext()));
        else requestPermissionForAlias("background", call, "backgroundDone");
    }

    @PermissionCallback
    private void backgroundDone(PluginCall call) {
        resolveGranted(call, hasBackground(getContext()));
    }

    /** {granted}: may Kinjo find nearby Kinjo phones over Bluetooth? ask:true shows Android's "Nearby devices" prompt. */
    @PluginMethod
    public void bluetooth(PluginCall call) {
        if (Ble.allowed(getContext()) || !call.getBoolean("ask", false)) resolveGranted(call, Ble.allowed(getContext()));
        else requestPermissionForAlias("bluetooth", call, "bluetoothDone");
    }

    @PermissionCallback
    private void bluetoothDone(PluginCall call) {
        resolveGranted(call, Ble.allowed(getContext()));
    }

    private void resolveGranted(PluginCall call, boolean granted) {
        JSObject out = new JSObject();
        out.put("granted", granted);
        call.resolve(out);
    }

    /** {unrestricted, maker}: is battery optimisation off for Kinjo, and who made the phone (for the right steps). */
    @PluginMethod
    public void battery(PluginCall call) {
        PowerManager pm = (PowerManager) getContext().getSystemService(Context.POWER_SERVICE);
        JSObject out = new JSObject();
        out.put("unrestricted", pm != null && pm.isIgnoringBatteryOptimizations(getContext().getPackageName()));
        out.put("maker", Build.MANUFACTURER.toLowerCase());
        call.resolve(out);
    }

    /** What the service is doing, for the "Location check" row. */
    @PluginMethod
    public void status(PluginCall call) {
        JSObject o = Status.json();
        o.put("background", hasBackground(getContext()));
        o.put("bluetoothAllowed", Ble.allowed(getContext()));
        call.resolve(o);
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

    /** Tell the open app something happened; a no-op when the app is closed. */
    static void emit(String event, JSObject data) {
        PresencePlugin p = live;
        if (p != null) p.notifyListeners(event, data);
    }
}
