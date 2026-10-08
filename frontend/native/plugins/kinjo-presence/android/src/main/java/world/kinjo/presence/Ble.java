package world.kinjo.presence;

import android.Manifest;
import android.annotation.SuppressLint;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothManager;
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
import java.util.Collections;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import org.json.JSONObject;

/**
 * Bluetooth, the second proximity signal (indoors GPS is often too vague). The phone
 * advertises a short random token from the server (never the uid) and listens for other
 * Kinjo phones; every REPORT_MS it sends the server what it heard. Android rules this
 * follows: the scan is filtered (unfiltered scans stop with the screen off), it is
 * restarted every RESCAN_MS (a scan left running 30 min gets downgraded) and never more
 * than 5 times in 30 s (Android silently ignores the rest).
 */
@SuppressLint("MissingPermission") // checked in allowed()
final class Ble {
    static final ParcelUuid KINJO = ParcelUuid.fromString("6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30");
    static final long REPORT_MS = 10_000, RESCAN_MS = 25 * 60_000, HEARD_FOR_MS = 120_000;

    private final Context ctx;
    private final Net net;
    private final Handler h = new Handler(Looper.getMainLooper());
    private final Map<String, Integer> batch = new HashMap<>(); // token -> strongest RSSI since the last report
    private final Map<String, Long> heardAt = new HashMap<>(); // token -> last heard (for the status count)
    private String token;
    private boolean on, fg, advertising, scanning;
    private long scanSince, tokenDue;

    private final AdvertiseCallback adv = new AdvertiseCallback() {
        @Override
        public void onStartFailure(int errorCode) {
            advertising = false;
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
    private final Runnable tick = new Runnable() {
        @Override
        public void run() {
            step();
            if (on) h.postDelayed(this, REPORT_MS);
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
        if (scanning) stopScan(); // restarts on the next step with the new mode (one start, well under 5/30 s)
    }

    void stop() {
        on = false;
        h.removeCallbacks(tick);
        stopAdvertising();
        stopScan();
        Status.bleOn = false;
        Status.bleHeard = 0;
    }

    /** Every REPORT_MS: follow Bluetooth on/off, keep the token fresh, restart scans, report. */
    private void step() {
        BluetoothAdapter a = adapter();
        boolean ready = a != null && a.isEnabled() && allowed(ctx);
        Status.bleOn = ready;
        if (!ready) { // off or not allowed: GPS carries on alone
            advertising = scanning = false;
            return;
        }
        long now = SystemClock.elapsedRealtime();
        if (token == null || now >= tokenDue) fetchToken();
        if (!advertising && token != null) startAdvertising(a);
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
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_BALANCED)
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
            .setScanMode(fg ? ScanSettings.SCAN_MODE_BALANCED : ScanSettings.SCAN_MODE_LOW_POWER)
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
        heardAt.put(t, SystemClock.elapsedRealtime());
    }

    private void report(long now) {
        for (Iterator<Long> it = heardAt.values().iterator(); it.hasNext(); ) if (now - it.next() > HEARD_FOR_MS) it.remove();
        Status.bleHeard = heardAt.size();
        if (batch.isEmpty()) return;
        StringBuilder b = new StringBuilder("{\"seen\":[");
        int n = 0;
        for (Map.Entry<String, Integer> e : batch.entrySet()) {
            if (n++ == 32) break; // the server's limit per report
            if (n > 1) b.append(',');
            b.append("{\"tok\":\"").append(e.getKey()).append("\",\"rssi\":").append(e.getValue()).append('}');
        }
        batch.clear();
        net.post("/api/sightings", b.append("]}").toString());
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
