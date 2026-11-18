package world.kinjo.presence;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Handler;
import android.os.Looper;
import android.os.PowerManager;
import androidx.annotation.NonNull;
import java.util.concurrent.TimeUnit;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;
import org.json.JSONObject;

/**
 * The service's own socket to the server (docs/proximity.md): fixes go up as "pos", the server
 * answers each with "mode"; "vis" false means hidden on another device. No pings either way
 * (each would wake the radio): the phone's own reports, at least every 2 min, keep it open.
 * Reconnects with backoff and jitter; while it's down, reports go over HTTP.
 */
final class Live extends WebSocketListener {
    private final Context ctx;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final PowerManager pm;
    private final OkHttpClient http = new OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // a socket waits as long as it needs
        .build();
    interface ModeListener {
        void mode(String mode, int next, int sleep);
    }

    private final ModeListener onMode;
    private final Runnable onHidden, onUnauthorized, onOpen;
    private volatile WebSocket ws;
    private volatile boolean open, stopped;
    private int retry;

    Live(Context ctx, ModeListener onMode, Runnable onHidden, Runnable onUnauthorized, Runnable onOpen) {
        this.ctx = ctx;
        this.pm = (PowerManager) ctx.getSystemService(Context.POWER_SERVICE);
        this.onMode = onMode;
        this.onHidden = onHidden;
        this.onUnauthorized = onUnauthorized;
        this.onOpen = onOpen;
    }

    void connect() {
        if (stopped || ws != null) return;
        SharedPreferences p = ctx.getSharedPreferences(PresenceService.PREFS, Context.MODE_PRIVATE);
        String api = p.getString("api", null), token = p.getString("token", null);
        if (api == null || token == null) return;
        Request req = new Request.Builder()
            .url(api.replaceFirst("^http", "ws") + "/ws")
            .header("Sec-WebSocket-Protocol", "kinjo, " + token + ", reporter") // the token rides in a header, never the URL
            .build();
        ws = http.newWebSocket(req, this);
    }

    /** False when the socket isn't open: the caller sends over HTTP instead. */
    boolean send(String json) {
        WebSocket w = ws;
        if (!open || w == null) return false;
        PowerManager.WakeLock wake = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "kinjo:live");
        wake.acquire(3_000); // timed: long enough for the frame to leave with the screen off
        return w.send(json);
    }

    boolean isOpen() {
        return open;
    }

    /** The network changed (Wi-Fi <-> mobile): the old socket is dead even if it doesn't know
     *  yet, and a still phone may not send for a long time. Dial again now. */
    void reconnect() {
        if (stopped) return;
        WebSocket w = ws;
        ws = null;
        open = false;
        Status.live = false;
        main.removeCallbacksAndMessages(null); // a pending backoff retry
        if (w != null) w.cancel();
        retry = 0;
        connect();
    }

    void stop() {
        stopped = true;
        main.removeCallbacksAndMessages(null);
        WebSocket w = ws;
        if (w != null) w.close(1000, null);
        http.dispatcher().executorService().shutdown();
    }

    @Override
    public void onOpen(@NonNull WebSocket w, @NonNull Response r) {
        main.post(() -> {
            if (w != ws) return;
            open = true;
            retry = 0;
            Status.live = true;
            onOpen.run(); // send the latest fix now: the server may have restarted
        });
    }

    @Override
    public void onMessage(@NonNull WebSocket w, @NonNull String text) {
        try {
            JSONObject o = new JSONObject(text);
            String type = o.optString("type");
            if ("mode".equals(type)) {
                String m = o.optString("mode", "near");
                int next = o.optInt("next", 15), sleep = o.optInt("sleep", 0);
                main.post(() -> onMode.mode(m, next, sleep));
            } else if ("vis".equals(type) && !o.optBoolean("visible", true)) {
                main.post(onHidden);
            }
        } catch (Exception e) {
            // not for us
        }
    }

    @Override
    public void onClosed(@NonNull WebSocket w, int code, @NonNull String reason) {
        main.post(() -> down(w, false));
    }

    @Override
    public void onFailure(@NonNull WebSocket w, @NonNull Throwable t, Response r) {
        boolean unauthorized = r != null && r.code() == 401;
        main.post(() -> down(w, unauthorized));
    }

    private void down(WebSocket w, boolean unauthorized) {
        if (w != ws) return;
        ws = null;
        open = false;
        Status.live = false;
        if (stopped) return;
        if (unauthorized) {
            onUnauthorized.run();
            return;
        }
        // 1 s, 2 s, 4 s … 60 s, with jitter: a server restart isn't hit by every phone at once
        long wait = (long) (Math.min(60_000, 1_000L << Math.min(retry++, 6)) * (0.5 + Math.random()));
        main.postDelayed(this::connect, wait);
    }
}
