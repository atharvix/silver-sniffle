package world.kinjo.presence;

import android.content.Context;
import android.net.wifi.ScanResult;
import android.net.wifi.WifiManager;
import android.os.SystemClock;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * The Wi-Fi networks around, for the server's same-place check (docs/proximity.md): from the
 * phone's last scan, which Android runs for its own location fixes. Never a scan of ours, so no
 * battery and no throttling. Works with Wi-Fi switched off ("Wi-Fi scanning" is on by default).
 * Each network goes out as the first 4 bytes of SHA-256 of its address; the server only
 * compares them, in memory.
 */
final class Wifi {
    static final int MAX = 12;
    static final long FRESH_MS = 120_000;

    /** A JSON array of hashed ids, strongest first, or "[]". */
    static String ids(Context c) {
        List<ScanResult> rs;
        try {
            WifiManager wm = (WifiManager) c.getApplicationContext().getSystemService(Context.WIFI_SERVICE);
            rs = wm == null ? null : wm.getScanResults();
        } catch (SecurityException e) {
            return "[]";
        }
        if (rs == null || rs.isEmpty()) return "[]";
        long nowUs = SystemClock.elapsedRealtime() * 1000;
        List<ScanResult> fresh = new ArrayList<>();
        for (ScanResult r : rs) {
            if (r.BSSID != null && nowUs - r.timestamp <= FRESH_MS * 1000) fresh.add(r);
        }
        fresh.sort((a, b) -> Integer.compare(b.level, a.level));
        StringBuilder out = new StringBuilder("[");
        for (int i = 0; i < Math.min(MAX, fresh.size()); i++) {
            if (i > 0) out.append(',');
            out.append('"').append(hash(fresh.get(i).BSSID)).append('"');
        }
        return out.append(']').toString();
    }

    private static String hash(String bssid) {
        try {
            byte[] d = MessageDigest.getInstance("SHA-256").digest(bssid.toLowerCase(Locale.US).getBytes(StandardCharsets.UTF_8));
            return String.format(Locale.US, "%02x%02x%02x%02x", d[0], d[1], d[2], d[3]);
        } catch (java.security.NoSuchAlgorithmException e) {
            return "00000000";
        }
    }
}
