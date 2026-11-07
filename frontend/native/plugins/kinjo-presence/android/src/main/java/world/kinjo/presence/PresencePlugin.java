package world.kinjo.presence;

import android.Manifest;
import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.PowerManager;
import android.provider.Settings;
import androidx.activity.result.ActivityResult;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.ActivityCallback;
import com.getcapacitor.annotation.CapacitorPlugin;
import com.getcapacitor.annotation.Permission;
import com.getcapacitor.annotation.PermissionCallback;

/** The app's handle on {@link PresenceService}: start/stop it, ask for permissions, read its status. */
@CapacitorPlugin(
    name = "KinjoPresence",
    permissions = {
        @Permission(alias = "location", strings = { Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION }),
        @Permission(alias = "bluetooth", strings = { Manifest.permission.BLUETOOTH_SCAN, Manifest.permission.BLUETOOTH_ADVERTISE, Manifest.permission.BLUETOOTH_CONNECT })
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
        Check.disarm(getContext());
        getContext().stopService(new Intent(getContext(), PresenceService.class));
        call.resolve();
    }

    /** The server's "scan" (socket): full-speed Bluetooth for 30 s and a report now. */
    @PluginMethod
    public void burst(PluginCall call) {
        PresenceService.poke(getContext(), true); // the app is open: a start is allowed
        call.resolve();
    }

    /** {granted, serve}: may Kinjo find nearby Kinjo phones? ask:true shows Android's "Nearby devices" prompt. */
    @PluginMethod
    public void bluetooth(PluginCall call) {
        boolean done = Ble.allowed(getContext()) && Ble.canServe(getContext());
        if (done || !call.getBoolean("ask", false) || Build.VERSION.SDK_INT < 31) resolveBluetooth(call);
        else requestPermissionForAlias("bluetooth", call, "bluetoothDone");
    }

    @PermissionCallback
    private void bluetoothDone(PluginCall call) {
        if (Ble.allowed(getContext())) PresenceService.start(getContext()); // granted while the app runs: start now
        resolveBluetooth(call);
    }

    private void resolveBluetooth(PluginCall call) {
        JSObject out = new JSObject();
        out.put("granted", Ble.allowed(getContext()));
        out.put("serve", Ble.canServe(getContext()));
        call.resolve(out);
    }

    /** {unrestricted, maker}; ask:true shows Android's "Let app always run in background?" dialog. */
    @PluginMethod
    public void battery(PluginCall call) {
        if (unrestricted() || !call.getBoolean("ask", false)) { resolveBattery(call); return; }
        Intent i = new Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, Uri.fromParts("package", getContext().getPackageName(), null));
        try { startActivityForResult(call, i, "batteryDone"); }
        catch (ActivityNotFoundException e) { openSettings(call); }
    }

    @ActivityCallback
    private void batteryDone(PluginCall call, ActivityResult result) {
        resolveBattery(call);
    }

    private boolean unrestricted() {
        PowerManager pm = (PowerManager) getContext().getSystemService(Context.POWER_SERVICE);
        return pm != null && pm.isIgnoringBatteryOptimizations(getContext().getPackageName());
    }

    private void resolveBattery(PluginCall call) {
        JSObject out = new JSObject();
        out.put("unrestricted", unrestricted());
        out.put("maker", Build.MANUFACTURER.toLowerCase());
        call.resolve(out);
    }

    /** What the service is doing right now. */
    @PluginMethod
    public void status(PluginCall call) {
        JSObject o = Status.json();
        o.put("bluetoothAllowed", Ble.allowed(getContext()));
        o.put("bluetoothServe", Ble.canServe(getContext()));
        o.put("located", Gps.located(getContext()));
        call.resolve(o);
    }

    /** Kinjo's page in Android settings. */
    @PluginMethod
    public void openSettings(PluginCall call) {
        Intent i = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", getContext().getPackageName(), null));
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        getContext().startActivity(i);
        call.resolve();
    }

    /** Tell the open app something happened; a no-op when the app is closed. */
    static void emit(String event, JSObject data) {
        PresencePlugin p = live;
        if (p != null) p.notifyListeners(event, data);
    }
}
