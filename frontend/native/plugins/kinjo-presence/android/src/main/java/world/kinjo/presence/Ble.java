package world.kinjo.presence;

import android.Manifest;
import android.annotation.SuppressLint;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothGatt;
import android.bluetooth.BluetoothGattCallback;
import android.bluetooth.BluetoothGattCharacteristic;
import android.bluetooth.BluetoothGattServer;
import android.bluetooth.BluetoothGattServerCallback;
import android.bluetooth.BluetoothGattService;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothProfile;
import android.bluetooth.BluetoothStatusCodes;
import android.bluetooth.le.AdvertiseCallback;
import android.bluetooth.le.AdvertiseData;
import android.bluetooth.le.AdvertiseSettings;
import android.bluetooth.le.BluetoothLeAdvertiser;
import android.bluetooth.le.BluetoothLeScanner;
import android.bluetooth.le.ScanCallback;
import android.bluetooth.le.ScanFilter;
import android.bluetooth.le.ScanRecord;
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
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import org.json.JSONObject;

/**
 * Bluetooth, the proximity signal (docs/proximity.md is the contract). The phone advertises a
 * random token from the server (never the uid), listens for other Kinjo phones and reports
 * what it heard; the server decides who is near.
 *
 * Two adverts: service data KINJO = token (Android to Android, and open iPhones), and a
 * connectable one listing KINJO, backed by a GATT server (TOKEN to read, SIGNAL to write) for
 * iPhones, which can't read service data in the background. Three scan filters: Android's
 * service data; the KINJO service UUID (an open iPhone carries its token in its local name);
 * Apple's overflow record with the iOS bit (a background iPhone: connect, read TOKEN, write our
 * own token and the iPhone tokens we heard directly, so iPhones learn about each other through
 * us). Up to MAX_CONNS connections at once (one at a time queued a crowd of iPhones for
 * minutes); a failed one waits 2, 5, 10, 20, 40 s, then IPHONE_RETRY_MS. In the background the
 * scan runs BALANCED (a quarter of the time): LOW_POWER (a tenth) left pocketed phones
 * finding each other in 10-20 s instead of a few.
 *
 * Android rules this follows: scans are filtered (unfiltered ones stop with the screen off),
 * restarted at most once per SCAN_GAP_MS (more than 5 starts in 30 s and Android silently
 * withholds results) and every RESCAN_MS (a scan left running 30 min gets downgraded).
 */
@SuppressLint("MissingPermission") // checked in allowed() / canServe()
final class Ble {
    static final ParcelUuid KINJO = ParcelUuid.fromString("6b1a0c30-4f9e-4b8e-9c55-2d3c7e1f0a30");
    static final UUID TOKEN_CHAR = UUID.fromString("6b1a0c31-4f9e-4b8e-9c55-2d3c7e1f0a30");
    static final UUID SIGNAL_CHAR = UUID.fromString("6b1a0c32-4f9e-4b8e-9c55-2d3c7e1f0a30");
    static final byte I_AM = 0x01, RELAY = 0x02;
    // a phone not heard for NEW_MS goes out at once; the rest is a refresh, sent every 20 s closed /
    // 10 s open (the server counts a hearing 45 s): at 10/5 s, 100k phones were ~10,000 requests/s
    static final long REPORT_MS = 20_000, REPORT_FG_MS = 10_000, RESCAN_MS = 25 * 60_000, SCAN_GAP_MS = 6_000;
    static final long HEARD_FOR_MS = 120_000, NEW_MS = 8_000, NEW_DELAY_MS = 300, BURST_MS = 30_000;
    static final long RELAY_MS = 30_000, IPHONE_CACHE_MS = 10 * 60_000, IPHONE_RETRY_MS = 60_000, IPHONE_READ_MS = 8_000;
    static final int APPLE = 0x004C, IOS_BIT = 101, MAX_RELAY = 10, MAX_REPORT = 32, MAX_CONNS = 3;

    private final Context ctx;
    private final Net net;
    private final Handler h = new Handler(Looper.getMainLooper());
    private final Map<String, Integer> batch = new HashMap<>(); // token -> strongest RSSI since the last report (null = over a connection)
    private final Map<String, Long> heardAt = new HashMap<>(); // token -> last heard
    private final Map<String, Long> iphoneHeard = new HashMap<>(); // iPhone token -> last heard directly (relayed to other iPhones)
    private final Map<String, String> iphoneTok = new HashMap<>(); // address -> token read over GATT
    private final Map<String, Long> iphoneAt = new HashMap<>(), iphoneFailed = new HashMap<>();
    private final Map<String, Integer> iphoneFails = new HashMap<>();
    private final Map<BluetoothGatt, Conn> conns = new HashMap<>(); // connections to iPhones in flight
    private String token;
    private volatile byte[] served; // the token as GATT reads answer it (binder thread)
    private volatile BluetoothGattServer gatt;
    private boolean on, fg, advertising, scanning, beacon, beaconBroken, scanFast;
    private long scanSince, tokenDue, burstUntil;
    private int iosBit = IOS_BIT;

    /** One connection to an iPhone: read its token, then write our SIGNAL frames. */
    private static final class Conn {
        final String addr;
        final int rssi;
        final ArrayDeque<byte[]> writes = new ArrayDeque<>();
        String tok;
        Runnable timeout;

        Conn(String addr, int rssi) {
            this.addr = addr;
            this.rssi = rssi;
        }
    }

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
            if (errorCode == ADVERTISE_FAILED_FEATURE_UNSUPPORTED) beaconBroken = true; // too many advertisers: retried next step
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
        public void onCharacteristicWriteRequest(BluetoothDevice d, int id, BluetoothGattCharacteristic c, boolean prepared, boolean needsReply, int offset, byte[] v) {
            BluetoothGattServer g = gatt;
            if (g != null && needsReply) try { g.sendResponse(d, id, BluetoothGatt.GATT_SUCCESS, offset, null); } catch (RuntimeException e) { }
            // an iPhone connected to us says who it is: heard over a connection (no RSSI). Relays are for iPhones.
            if (SIGNAL_CHAR.equals(c.getUuid()) && v != null && v.length == 9 && v[0] == I_AM) {
                String t = hex(Arrays.copyOfRange(v, 1, 9));
                h.post(() -> { if (on) heardToken(t, null); });
            }
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
        public void onBatchScanResults(List<ScanResult> rs) {
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
            if (on) h.postDelayed(this, fast() ? REPORT_FG_MS : REPORT_MS);
        }
    };
    private final Runnable rescan = () -> { // scan in the mode we want now, not on the next tick
        stopScan();
        h.removeCallbacks(this.tick);
        this.tick.run();
    };
    // binder threads: everything goes back to h
    private final BluetoothGattCallback client = new BluetoothGattCallback() {
        @Override
        public void onConnectionStateChange(BluetoothGatt g, int status, int state) {
            if (status == BluetoothGatt.GATT_SUCCESS && state == BluetoothProfile.STATE_CONNECTED) {
                if (!g.discoverServices()) h.post(() -> endRead(g, false));
            } else h.post(() -> endRead(g, false));
        }

        @Override
        public void onServicesDiscovered(BluetoothGatt g, int status) {
            BluetoothGattCharacteristic c = status == BluetoothGatt.GATT_SUCCESS ? characteristic(g, TOKEN_CHAR) : null;
            if (c == null || !g.readCharacteristic(c)) h.post(() -> endRead(g, false));
        }

        @Override
        public void onCharacteristicRead(BluetoothGatt g, BluetoothGattCharacteristic c, byte[] v, int status) { // Android 13+
            gotToken(g, v, status);
        }

        @Override
        @SuppressWarnings("deprecation")
        public void onCharacteristicRead(BluetoothGatt g, BluetoothGattCharacteristic c, int status) { // before 13
            gotToken(g, c.getValue(), status);
        }

        @Override
        public void onCharacteristicWrite(BluetoothGatt g, BluetoothGattCharacteristic c, int status) {
            h.post(() -> { if (conns.containsKey(g)) nextWrite(g); });
        }

        private void gotToken(BluetoothGatt g, byte[] v, int status) {
            String t = status == BluetoothGatt.GATT_SUCCESS && v != null && v.length == 8 ? hex(v) : null;
            h.post(() -> { if (t == null) endRead(g, false); else gotIphone(g, t); });
        }
    };

    Ble(Context ctx, Net net) {
        this.ctx = ctx;
        this.net = net;
        net.onConflict = () -> { // the server doesn't know our token: nobody hears us until we fetch one
            tokenDue = 0;
            if (on) step();
        };
    }

    /** May Kinjo scan and advertise? Android 12+: "Nearby devices" (no location: neverForLocation). Older: location. */
    static boolean allowed(Context c) {
        if (Build.VERSION.SDK_INT < 31) return Gps.located(c);
        return granted(c, Manifest.permission.BLUETOOTH_SCAN) && granted(c, Manifest.permission.BLUETOOTH_ADVERTISE);
    }

    /** May iPhones connect to us, and may we connect to them? (Android 12+: BLUETOOTH_CONNECT, same group.) */
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

    /** The app came to the screen or left it: the scan changes speed at once. */
    void foreground(boolean v) {
        if (fg == v) return;
        fg = v;
        if (on) rescanSoon();
    }

    /** A wake push or the server's "scan": full speed for BURST_MS and a report now. */
    void burst() {
        if (!on) return;
        burstUntil = SystemClock.elapsedRealtime() + BURST_MS;
        rescanSoon();
        h.postDelayed(() -> { if (on && !fast()) rescanSoon(); }, BURST_MS + 100);
    }

    void stop() {
        on = false;
        h.removeCallbacksAndMessages(null);
        for (BluetoothGatt g : new ArrayList<>(conns.keySet())) endRead(g, false);
        iphoneTok.clear(); iphoneAt.clear(); iphoneFailed.clear(); iphoneFails.clear(); iphoneHeard.clear();
        batch.clear(); heardAt.clear();
        stopAdvertising();
        stopBeacon();
        closeGatt();
        beaconBroken = false;
        stopScan();
        token = null;
        served = null;
        tokenDue = 0;
        Status.bleOn = false;
        Status.bleHeard = 0;
    }

    private boolean fast() {
        return fg || SystemClock.elapsedRealtime() < burstUntil;
    }

    private void rescanSoon() {
        long wait = scanning ? SCAN_GAP_MS - (SystemClock.elapsedRealtime() - scanSince) : 0;
        h.removeCallbacks(rescan);
        h.postDelayed(rescan, Math.max(0, wait));
    }

    /** Every report interval: follow Bluetooth on/off, keep the token fresh, restart scans, report. */
    private void step() {
        if (!on) return;
        BluetoothAdapter a = adapter();
        boolean ready = a != null && a.isEnabled() && allowed(ctx);
        Status.bleOn = ready;
        if (!ready) {
            advertising = scanning = beacon = false;
            closeGatt(); // a server opened before Bluetooth went off is dead
            return;
        }
        long now = SystemClock.elapsedRealtime();
        if (token == null || now >= tokenDue) fetchToken();
        if (!advertising && token != null) startAdvertising(a);
        if (advertising && !beacon && !beaconBroken && canServe(ctx)) startBeacon(a);
        if (scanning && (now - scanSince > RESCAN_MS || (scanFast != fast() && now - scanSince > SCAN_GAP_MS))) stopScan();
        if (!scanning) startScan(a);
        report(now);
    }

    private void fetchToken() {
        tokenDue = SystemClock.elapsedRealtime() + 60_000; // retry in a minute if this fails
        net.get("/api/ble-token", body -> {
            try {
                JSONObject o = new JSONObject(body);
                String t = o.getString("token");
                int bit = o.optInt("iosBit", IOS_BIT);
                if (bit != iosBit) { iosBit = bit; stopScan(); }
                tokenDue = SystemClock.elapsedRealtime() + Math.max(60, o.optInt("every", 900) - 30) * 1000L;
                if (isToken(t) && !t.equals(token)) {
                    token = t;
                    served = hex(t);
                    stopAdvertising(); // back up with the new token on this step
                    h.post(this::step);
                }
            } catch (Exception e) {
                // malformed: keep the old token, retry at tokenDue
            }
        });
    }

    private void startAdvertising(BluetoothAdapter a) {
        BluetoothLeAdvertiser ad = a.getBluetoothLeAdvertiser();
        if (ad == null) return; // can't advertise; can still hear others
        AdvertiseSettings s = new AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY) // done by the radio, keeps going while the phone sleeps
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

    /** Connectable advert listing KINJO, for iPhones to find and connect to. */
    private void startBeacon(BluetoothAdapter a) {
        BluetoothLeAdvertiser ad = a.getBluetoothLeAdvertiser();
        if (ad == null || !a.isMultipleAdvertisementSupported() || !openGatt()) return;
        AdvertiseSettings s = new AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_BALANCED)
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

    private void stopAdvertising() {
        BluetoothAdapter a = adapter();
        if (advertising && a != null && a.isEnabled() && a.getBluetoothLeAdvertiser() != null) {
            try { a.getBluetoothLeAdvertiser().stopAdvertising(adv); } catch (RuntimeException e) { }
        }
        advertising = false;
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
            s.addCharacteristic(new BluetoothGattCharacteristic(SIGNAL_CHAR,
                BluetoothGattCharacteristic.PROPERTY_WRITE | BluetoothGattCharacteristic.PROPERTY_WRITE_NO_RESPONSE, BluetoothGattCharacteristic.PERMISSION_WRITE));
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

    private void startScan(BluetoothAdapter a) {
        BluetoothLeScanner sc = a.getBluetoothLeScanner();
        if (sc == null) return;
        // any 8-byte token under our UUID: an all-zero mask means "don't compare these bytes"
        List<ScanFilter> fs = new ArrayList<>();
        fs.add(new ScanFilter.Builder().setServiceData(KINJO, new byte[8], new byte[8]).build());
        fs.add(new ScanFilter.Builder().setServiceUuid(KINJO).build());
        if (iosBit >= 0 && iosBit < 128 && canServe(ctx)) { // background iPhones: Apple's 0x01 overflow record with our bit set
            byte[] d = new byte[17], m = new byte[17];
            d[0] = 1;
            m[0] = (byte) 0xFF;
            d[1 + iosBit / 8] = m[1 + iosBit / 8] = (byte) (0x80 >>> (iosBit % 8));
            fs.add(new ScanFilter.Builder().setManufacturerData(APPLE, d, m).build());
        }
        boolean f = fast();
        ScanSettings s = new ScanSettings.Builder()
            .setScanMode(f ? ScanSettings.SCAN_MODE_LOW_LATENCY : ScanSettings.SCAN_MODE_BALANCED)
            .build();
        try {
            sc.startScan(fs, s, scan);
            scanning = true;
            scanFast = f;
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
        if (!on) return;
        ScanRecord rec = r.getScanRecord();
        if (rec == null) return;
        byte[] d = rec.getServiceData(KINJO);
        String name = rec.getDeviceName();
        if (d != null && d.length == 8) heardToken(hex(d), r.getRssi()); // an Android phone
        else if (isToken(name)) { iphoneHeard.put(name, SystemClock.elapsedRealtime()); heardToken(name, r.getRssi()); } // an open iPhone
        else if (iosBitSet(rec.getManufacturerSpecificData(APPLE))) iphone(r.getDevice(), r.getRssi()); // a background iPhone
    }

    private boolean iosBitSet(byte[] m) {
        return iosBit >= 0 && iosBit < 128 && m != null && m.length >= 17 && m[0] == 1
            && (m[1 + iosBit / 8] & (0x80 >>> (iosBit % 8))) != 0;
    }

    /** A background iPhone: its token from the cache, else connect (up to MAX_CONNS at once). */
    private void iphone(BluetoothDevice dev, int rssi) {
        String a = dev.getAddress();
        long now = SystemClock.elapsedRealtime();
        String t = iphoneTok.get(a);
        Long at = iphoneAt.get(a), failed = iphoneFailed.get(a);
        if (t != null && at != null && now - at < IPHONE_CACHE_MS) {
            iphoneHeard.put(t, now);
            heardToken(t, rssi);
            return;
        }
        if (conns.size() >= MAX_CONNS || (failed != null && now - failed < retryWait(a)) || !canServe(ctx)) return;
        for (Conn c : conns.values()) if (c.addr.equals(a)) return; // already connecting
        BluetoothGatt g;
        try {
            g = dev.connectGatt(ctx, false, client, BluetoothDevice.TRANSPORT_LE);
        } catch (RuntimeException e) { // SecurityException: CONNECT revoked
            g = null;
        }
        if (g == null) { failed(a, now); return; }
        Conn c = new Conn(a, rssi);
        BluetoothGatt gg = g;
        c.timeout = () -> endRead(gg, false);
        conns.put(g, c); // callbacks reach us through h, after this
        h.postDelayed(c.timeout, IPHONE_READ_MS);
    }

    /** Read the iPhone's token: report it, then tell it who we are and which iPhones we hear. */
    private void gotIphone(BluetoothGatt g, String t) {
        Conn c = conns.get(g);
        if (c == null) { close(g); return; }
        c.tok = t;
        long now = SystemClock.elapsedRealtime();
        iphoneTok.put(c.addr, t);
        iphoneAt.put(c.addr, now);
        iphoneHeard.put(t, now);
        heardToken(t, c.rssi);
        if (token != null) c.writes.add(frame(I_AM, token));
        int n = 0;
        for (Map.Entry<String, Long> e : iphoneHeard.entrySet()) {
            if (n == MAX_RELAY) break;
            if (!e.getKey().equals(t) && now - e.getValue() <= RELAY_MS) { c.writes.add(frame(RELAY, e.getKey())); n++; }
        }
        nextWrite(g);
    }

    @SuppressWarnings("deprecation")
    private void nextWrite(BluetoothGatt g) {
        BluetoothGattCharacteristic c = characteristic(g, SIGNAL_CHAR);
        Conn cn = conns.get(g);
        byte[] f = cn == null ? null : cn.writes.poll();
        if (c == null || f == null) { endRead(g, true); return; } // done, or an old build without SIGNAL
        boolean ok;
        try {
            if (Build.VERSION.SDK_INT >= 33) ok = g.writeCharacteristic(c, f, BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT) == BluetoothStatusCodes.SUCCESS;
            else { c.setValue(f); c.setWriteType(BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT); ok = g.writeCharacteristic(c); }
        } catch (RuntimeException e) {
            ok = false;
        }
        if (!ok) endRead(g, true); // the token is in; the frames are a bonus
    }

    /** Main thread. Ends a connection (stale callbacks from a finished one just close it). */
    private void endRead(BluetoothGatt g, boolean ok) {
        if (g == null) return;
        Conn c = conns.remove(g);
        close(g);
        if (c == null) return;
        h.removeCallbacks(c.timeout);
        if (!ok && c.tok == null) { failed(c.addr, SystemClock.elapsedRealtime()); return; }
        iphoneFailed.remove(c.addr);
        iphoneFails.remove(c.addr);
    }

    private void failed(String a, long now) {
        iphoneFailed.put(a, now);
        Integer n = iphoneFails.get(a);
        iphoneFails.put(a, n == null ? 1 : n + 1);
    }

    /** 2, 5, 10, 20, 40 s, then IPHONE_RETRY_MS: Android connects often fail once (status 133) and work the next time. */
    private long retryWait(String a) {
        Integer n = iphoneFails.get(a);
        if (n == null || n <= 1) return 2_000;
        return Math.min(IPHONE_RETRY_MS, 5_000L << Math.min(4, n - 2));
    }

    private static BluetoothGattCharacteristic characteristic(BluetoothGatt g, UUID id) {
        BluetoothGattService s = g.getService(KINJO.getUuid());
        return s == null ? null : s.getCharacteristic(id);
    }

    private static void close(BluetoothGatt g) {
        try { g.disconnect(); g.close(); } catch (RuntimeException e) { }
    }

    /** rssi null: heard over a connection (an iPhone wrote to us), no signal strength. */
    private void heardToken(String t, Integer rssi) {
        if (t.equals(token)) return;
        Integer prev = batch.get(t);
        if (!batch.containsKey(t) || (rssi != null && (prev == null || rssi > prev))) batch.put(t, rssi);
        long now = SystemClock.elapsedRealtime();
        Long last = heardAt.put(t, now);
        if (last == null || now - last > NEW_MS) { // someone new: don't wait for the next report
            h.removeCallbacks(reportNew);
            h.postDelayed(reportNew, NEW_DELAY_MS);
        }
    }

    private void report(long now) {
        for (Iterator<Long> it = heardAt.values().iterator(); it.hasNext(); ) if (now - it.next() > HEARD_FOR_MS) it.remove();
        for (Iterator<Long> it = iphoneHeard.values().iterator(); it.hasNext(); ) if (now - it.next() > RELAY_MS) it.remove();
        for (Iterator<Map.Entry<String, Long>> it = iphoneAt.entrySet().iterator(); it.hasNext(); ) {
            Map.Entry<String, Long> e = it.next();
            if (now - e.getValue() > IPHONE_CACHE_MS) { iphoneTok.remove(e.getKey()); it.remove(); }
        }
        for (Iterator<Map.Entry<String, Long>> it = iphoneFailed.entrySet().iterator(); it.hasNext(); ) {
            Map.Entry<String, Long> e = it.next();
            if (now - e.getValue() > IPHONE_CACHE_MS) { iphoneFails.remove(e.getKey()); it.remove(); } // a non-Kinjo device stays at IPHONE_RETRY_MS
        }
        Status.bleHeard = heardAt.size();
        StringBuilder b = null;
        int n = 0;
        for (Map.Entry<String, Integer> e : batch.entrySet()) {
            if (b == null) b = new StringBuilder("{\"seen\":[");
            else b.append(',');
            b.append("{\"tok\":\"").append(e.getKey()).append('"');
            if (e.getValue() != null) b.append(",\"rssi\":").append(e.getValue());
            b.append('}');
            if (++n % MAX_REPORT == 0) { net.post("/api/sightings", b.append("]}").toString()); b = null; }
        }
        if (b != null) net.post("/api/sightings", b.append("]}").toString());
        batch.clear();
    }

    private BluetoothAdapter adapter() {
        BluetoothManager m = (BluetoothManager) ctx.getSystemService(Context.BLUETOOTH_SERVICE);
        return m == null ? null : m.getAdapter();
    }

    private static byte[] frame(byte type, String tok) {
        byte[] f = new byte[9];
        f[0] = type;
        System.arraycopy(hex(tok), 0, f, 1, 8);
        return f;
    }

    static boolean isToken(String s) {
        return s != null && s.matches("[0-9a-f]{16}");
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
