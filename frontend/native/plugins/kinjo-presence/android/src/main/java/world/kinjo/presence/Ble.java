package world.kinjo.presence;

import android.Manifest;
import android.annotation.SuppressLint;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothGatt;
import android.bluetooth.BluetoothGattCharacteristic;
import android.bluetooth.BluetoothGattServer;
import android.bluetooth.BluetoothGattServerCallback;
import android.bluetooth.BluetoothGattService;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothProfile;
import android.bluetooth.le.AdvertiseCallback;
import android.bluetooth.le.AdvertiseData;
import android.bluetooth.le.AdvertiseSettings;
import android.bluetooth.le.BluetoothLeAdvertiser;
import android.bluetooth.le.BluetoothLeScanner;
import android.bluetooth.le.ScanCallback;
import android.bluetooth.le.ScanFilter;
import android.bluetooth.le.ScanResult;
import android.bluetooth.le.ScanSettings;
import android.content.Context;
import android.content.pm.PackageManager;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelUuid;
import android.os.SystemClock;
import androidx.core.content.ContextCompat;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.UUID;
import org.json.JSONObject;

/**
 * Bluetooth, the second proximity signal (indoors GPS is often too vague). The phone
 * advertises a short random token from the server (never the uid) and listens for other
 * Kinjo phones; every REPORT_MS it sends the server what it heard, and a phone it hasn't
 * heard for NEW_MS goes out within NEW_DELAY_MS (that's what puts a new card on a deck,
 * one small request per arrival). Android rules this
 * follows: the scan is filtered (unfiltered scans stop with the screen off), it is
 * restarted every RESCAN_MS (a scan left running 30 min gets downgraded) and never more
 * than 5 times in 30 s (Android silently ignores the rest).
 *
 * Advertising is done by the Bluetooth chip and keeps going while the phone sleeps;
 * listening is what Android cuts back with the screen off. Since one phone hearing the
 * other counts for both (server side), the open phone listens at full speed and the
 * sleeping ones only need to advertise.
 *
 * iPhones can't read that advert in the background (iOS only reports connectable adverts
 * that list the service UUID) and can't advertise service data themselves. So, when
 * "Nearby devices" covers BLUETOOTH_CONNECT, a second, connectable advert lists KINJO and a
 * GATT server answers reads of TOKEN_CHAR with the same token: iPhones connect, read and
 * report it, and one side hearing counts for both. Android scanners never match that advert
 * (no service data), so Android to Android works exactly as before.
 */
@SuppressLint("MissingPermission") // checked in allowed()
final class Ble {
    static final ParcelUuid KINJO = ParcelUuid.fromString("6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30");
    static final UUID TOKEN_CHAR = UUID.fromString("6b1a0c31-4f9e-4b8e-9c55-2d3c7e1f0a30"); // read: the 8-byte token (for iPhones)
    static final long REPORT_MS = 10_000, REPORT_FG_MS = 5_000, RESCAN_MS = 25 * 60_000, HEARD_FOR_MS = 120_000;
    static final long NEW_MS = 20_000, NEW_DELAY_MS = 300; // under the server's 30 s pair window; batches a group arriving together

    private final Context ctx;
    private final Net net;
    private final Handler h = new Handler(Looper.getMainLooper());
    private final Map<String, Integer> batch = new HashMap<>(); // token -> strongest RSSI since the last report
    private final Map<String, Long> heardAt = new HashMap<>(); // token -> last heard (for the status count)
    private String token;
    private volatile byte[] served; // the token as GATT reads answer it (binder thread)
    private volatile BluetoothGattServer gatt;
    private boolean on, fg, advertising, scanning, beacon, beaconBroken;
    private long scanSince, tokenDue;

    private final AdvertiseCallback adv = new AdvertiseCallback() {
        @Override
        public void onStartFailure(int errorCode) {
            advertising = false;
        }
    };
    private final AdvertiseCallback beaconAdv = new AdvertiseCallback() {
        @Override
        public void onStartFailure(int errorCode) {
            beacon = false;
            // no slot for a second advert on this phone: stay Android-only, don't retry every step
            if (errorCode == ADVERTISE_FAILED_TOO_MANY_ADVERTISERS || errorCode == ADVERTISE_FAILED_FEATURE_UNSUPPORTED) beaconBroken = true;
        }
    };
    private final BluetoothGattServerCallback serve = new BluetoothGattServerCallback() {
        @Override
        public void onCharacteristicReadRequest(BluetoothDevice d, int id, int offset, BluetoothGattCharacteristic c) {
            BluetoothGattServer g = gatt;
            byte[] v = served;
            if (g == null) return;
            try {
                if (!TOKEN_CHAR.equals(c.getUuid()) || v == null) g.sendResponse(d, id, BluetoothGatt.GATT_FAILURE, offset, null);
                else if (offset > v.length) g.sendResponse(d, id, BluetoothGatt.GATT_INVALID_OFFSET, offset, null);
                else g.sendResponse(d, id, BluetoothGatt.GATT_SUCCESS, offset, Arrays.copyOfRange(v, offset, v.length));
            } catch (RuntimeException e) { }
        }

        @Override
        public void onConnectionStateChange(BluetoothDevice d, int status, int state) {
            // a connection can end a connectable advert on some stacks: put it back up
            if (state == BluetoothProfile.STATE_DISCONNECTED) h.post(() -> { stopBeacon(); if (on) step(); });
        }
    };
    private final ScanCallback scan = new ScanCallback() {
        @Override
        public void onScanResult(int type, ScanResult r) {
            heard(r);
        }

        @Override
        public void onBatchScanResults(java.util.List<ScanResult> rs) {
            for (ScanResult r : rs) heard(r);
        }

        @Override
        public void onScanFailed(int errorCode) {
            scanning = false;
        }
    };
    private final Runnable reportNew = () -> { if (on) report(SystemClock.elapsedRealtime()); };
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            step();
            if (on) h.postDelayed(this, fg ? REPORT_FG_MS : REPORT_MS);
        }
    };

    Ble(Context ctx, Net net) {
        this.ctx = ctx;
        this.net = net;
    }

    static boolean allowed(Context c) {
        if (Build.VERSION.SDK_INT < 31) return true; // older Android: covered by location permission
        return granted(c, Manifest.permission.BLUETOOTH_SCAN) && granted(c, Manifest.permission.BLUETOOTH_ADVERTISE);
    }

    /** May iPhones connect and read the token? (Android 12+: BLUETOOTH_CONNECT, same "Nearby devices" group.) */
    static boolean canServe(Context c) {
        return Build.VERSION.SDK_INT < 31 || granted(c, Manifest.permission.BLUETOOTH_CONNECT);
    }

    private static boolean granted(Context c, String p) {
        return ContextCompat.checkSelfPermission(c, p) == PackageManager.PERMISSION_GRANTED;
    }

    void start(boolean foreground) {
        fg = foreground;
        if (on) return;
        on = true;
        h.post(tick);
    }

    /** Scan harder while the app is on screen; the mode only changes on the next restart. */
    void foreground(boolean v) {
        if (fg == v) return;
        fg = v;
        // restarts on the next step with the new mode; at most one start per 6 s, or quick
        // screen toggles pass Android's 5 starts per 30 s and it silently withholds results
        if (scanning && SystemClock.elapsedRealtime() - scanSince > 6_000) stopScan();
    }

    /** The server's wake push: refresh the token if due, re-advertise, report now. */
    void kick() {
        if (on) step();
    }

    void stop() {
        on = false;
        h.removeCallbacks(tick);
        h.removeCallbacks(reportNew);
        stopAdvertising();
        stopBeacon();
        closeGatt();
        beaconBroken = false;
        stopScan();
        Status.bleOn = false;
        Status.bleHeard = 0;
    }

    /** Every REPORT_MS: follow Bluetooth on/off, keep the token fresh, restart scans, report. */
    private void step() {
        if (!on) return; // a token answer that lands after stop/hide must not advertise again
        BluetoothAdapter a = adapter();
        boolean ready = a != null && a.isEnabled() && allowed(ctx);
        Status.bleOn = ready;
        if (!ready) { // off or not allowed: GPS carries on alone
            advertising = scanning = beacon = false;
            closeGatt(); // a server opened before Bluetooth went off is dead
            return;
        }
        long now = SystemClock.elapsedRealtime();
        if (token == null || now >= tokenDue) fetchToken();
        if (!advertising && token != null) startAdvertising(a);
        if (advertising && !beacon && !beaconBroken && canServe(ctx)) startBeacon(a); // after the Android advert: it keeps the first slot
        if (scanning && now - scanSince > RESCAN_MS) stopScan();
        if (!scanning) startScan(a);
        report(now);
    }

    private void fetchToken() {
        tokenDue = SystemClock.elapsedRealtime() + 60_000; // retry in a minute if this fails
        net.get("/api/ble-token", body -> {
            try {
                JSONObject o = new JSONObject(body);
                String t = o.getString("token");
                tokenDue = SystemClock.elapsedRealtime() + Math.max(60, o.optInt("every", 900) - 30) * 1000L;
                if (!t.equals(token)) {
                    token = t;
                    served = hex(t); // GATT reads answer the new token at once; the beacon advert carries none
                    stopAdvertising(); // re-advertised with the new token on the next step
                    h.post(this::step);
                }
            } catch (Exception e) {
                // malformed answer: keep the old token, retry at tokenDue
            }
        });
    }

    private void startAdvertising(BluetoothAdapter a) {
        BluetoothLeAdvertiser ad = a.getBluetoothLeAdvertiser();
        if (ad == null) return; // this phone can't advertise; it can still hear others
        AdvertiseSettings s = new AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY) // ~10/s: done by the radio, keeps going while the phone sleeps
            .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_HIGH)
            .setConnectable(false)
            .build();
        AdvertiseData d = new AdvertiseData.Builder()
            .setIncludeDeviceName(false)
            .setIncludeTxPowerLevel(false)
            .addServiceData(KINJO, hex(token))
            .build();
        try {
            ad.startAdvertising(s, d, adv);
            advertising = true;
        } catch (RuntimeException e) {
            advertising = false;
        }
    }

    /** Connectable advert listing KINJO (flags + 128-bit UUID = 21 of 31 bytes), for iPhones to find and connect to. */
    private void startBeacon(BluetoothAdapter a) {
        BluetoothLeAdvertiser ad = a.getBluetoothLeAdvertiser();
        if (ad == null || !a.isMultipleAdvertisementSupported() || !openGatt()) return;
        AdvertiseSettings s = new AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_BALANCED) // ~4/s: background iPhones scan in slow windows anyway
            .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_HIGH)
            .setConnectable(true)
            .build();
        AdvertiseData d = new AdvertiseData.Builder()
            .setIncludeDeviceName(false)
            .setIncludeTxPowerLevel(false)
            .addServiceUuid(KINJO)
            .build();
        try {
            ad.startAdvertising(s, d, beaconAdv);
            beacon = true;
        } catch (RuntimeException e) {
            beacon = false;
        }
    }

    private void stopBeacon() {
        BluetoothAdapter a = adapter();
        if (beacon && a != null && a.isEnabled() && a.getBluetoothLeAdvertiser() != null) {
            try { a.getBluetoothLeAdvertiser().stopAdvertising(beaconAdv); } catch (RuntimeException e) { }
        }
        beacon = false;
    }

    private boolean openGatt() {
        if (gatt != null) return true;
        BluetoothManager m = (BluetoothManager) ctx.getSystemService(Context.BLUETOOTH_SERVICE);
        if (m == null) return false;
        try {
            BluetoothGattServer g = m.openGattServer(ctx, serve);
            if (g == null) return false;
            BluetoothGattService s = new BluetoothGattService(KINJO.getUuid(), BluetoothGattService.SERVICE_TYPE_PRIMARY);
            s.addCharacteristic(new BluetoothGattCharacteristic(TOKEN_CHAR, BluetoothGattCharacteristic.PROPERTY_READ, BluetoothGattCharacteristic.PERMISSION_READ));
            g.addService(s);
            if (token != null) served = hex(token);
            gatt = g;
            return true;
        } catch (RuntimeException e) { // SecurityException: CONNECT revoked
            return false;
        }
    }

    private void closeGatt() {
        BluetoothGattServer g = gatt;
        gatt = null;
        if (g != null) try { g.close(); } catch (RuntimeException e) { }
    }

    private void stopAdvertising() {
        BluetoothAdapter a = adapter();
        if (advertising && a != null && a.isEnabled() && a.getBluetoothLeAdvertiser() != null) {
            try { a.getBluetoothLeAdvertiser().stopAdvertising(adv); } catch (RuntimeException e) { }
        }
        advertising = false;
    }

    private void startScan(BluetoothAdapter a) {
        BluetoothLeScanner sc = a.getBluetoothLeScanner();
        if (sc == null) return;
        // any 8-byte token under our UUID: an all-zero mask means "don't compare these bytes" (documented),
        // whereas an empty data array is undocumented and some chips' hardware filters mishandle it
        ScanFilter f = new ScanFilter.Builder().setServiceData(KINJO, new byte[8], new byte[8]).build();
        ScanSettings s = new ScanSettings.Builder()
            .setScanMode(fg ? ScanSettings.SCAN_MODE_LOW_LATENCY : ScanSettings.SCAN_MODE_LOW_POWER)
            .build();
        try {
            sc.startScan(Collections.singletonList(f), s, scan);
            scanning = true;
            scanSince = SystemClock.elapsedRealtime();
        } catch (RuntimeException e) {
            scanning = false;
        }
    }

    private void stopScan() {
        BluetoothAdapter a = adapter();
        if (scanning && a != null && a.isEnabled() && a.getBluetoothLeScanner() != null) {
            try { a.getBluetoothLeScanner().stopScan(scan); } catch (RuntimeException e) { }
        }
        scanning = false;
    }

    private void heard(ScanResult r) {
        if (r.getScanRecord() == null) return;
        byte[] d = r.getScanRecord().getServiceData(KINJO);
        if (d == null || d.length != 8) return;
        String t = hex(d);
        if (t.equals(token)) return;
        Integer prev = batch.get(t);
        if (prev == null || r.getRssi() > prev) batch.put(t, r.getRssi());
        long now = SystemClock.elapsedRealtime();
        Long last = heardAt.put(t, now);
        if (last == null || now - last > NEW_MS) { // someone new: don't wait for the next report
            h.removeCallbacks(reportNew);
            h.postDelayed(reportNew, NEW_DELAY_MS);
        }
    }

    private void report(long now) {
        for (Iterator<Long> it = heardAt.values().iterator(); it.hasNext(); ) if (now - it.next() > HEARD_FOR_MS) it.remove();
        Status.bleHeard = heardAt.size();
        // 32 per request (the server's limit), every token heard: in a room of 60, dropping
        // the rest left the same people unreported every round
        StringBuilder b = null;
        int n = 0;
        for (Map.Entry<String, Integer> e : batch.entrySet()) {
            if (b == null) b = new StringBuilder("{\"seen\":[");
            else b.append(',');
            b.append("{\"tok\":\"").append(e.getKey()).append("\",\"rssi\":").append(e.getValue()).append('}');
            if (++n % 32 == 0) { net.post("/api/sightings", b.append("]}").toString()); b = null; }
        }
        if (b != null) net.post("/api/sightings", b.append("]}").toString());
        batch.clear();
    }

    private BluetoothAdapter adapter() {
        BluetoothManager m = (BluetoothManager) ctx.getSystemService(Context.BLUETOOTH_SERVICE);
        return m == null ? null : m.getAdapter();
    }

    private static byte[] hex(String s) {
        byte[] out = new byte[s.length() / 2];
        for (int i = 0; i < out.length; i++) out[i] = (byte) Integer.parseInt(s.substring(2 * i, 2 * i + 2), 16);
        return out;
    }

    private static String hex(byte[] b) {
        StringBuilder s = new StringBuilder();
        for (byte x : b) s.append(String.format("%02x", x));
        return s.toString();
    }
}
