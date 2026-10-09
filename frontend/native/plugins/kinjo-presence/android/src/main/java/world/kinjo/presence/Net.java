package world.kinjo.presence;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Handler;
import android.os.Looper;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Consumer;

/** Talks to the Kinjo API with the saved session, one request at a time, off the main thread. */
final class Net {
    private final Context ctx;
    private final Runnable onUnauthorized;
    private final ExecutorService ex = Executors.newSingleThreadExecutor();
    private final Handler main = new Handler(Looper.getMainLooper());

    Net(Context ctx, Runnable onUnauthorized) {
        this.ctx = ctx;
        this.onUnauthorized = onUnauthorized;
    }

    void post(String path, String json) {
        ex.execute(() -> send("POST", path, json, null));
    }

    /** GET; onBody runs on the main thread with the response body, only on 200. */
    void get(String path, Consumer<String> onBody) {
        ex.execute(() -> send("GET", path, null, onBody));
    }

    void shutdown() {
        ex.shutdown();
    }

    private void send(String method, String path, String json, Consumer<String> onBody) {
        SharedPreferences p = ctx.getSharedPreferences(PresenceService.PREFS, Context.MODE_PRIVATE);
        String api = p.getString("api", null), token = p.getString("token", null);
        if (api == null || token == null) return;
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(api + path).openConnection();
            c.setConnectTimeout(10_000);
            c.setReadTimeout(10_000);
            c.setRequestMethod(method);
            c.setRequestProperty("Authorization", "Bearer " + token);
            if (json != null) {
                c.setDoOutput(true);
                c.setRequestProperty("Content-Type", "application/json");
                try (OutputStream out = c.getOutputStream()) {
                    out.write(json.getBytes(StandardCharsets.UTF_8));
                }
            }
            int code = c.getResponseCode();
            Status.server(code);
            if (code < 300 && path.equals("/api/presence")) Status.sentAt = System.currentTimeMillis();
            if (code == 401) { // session gone (signed out elsewhere): stop for good
                main.post(onUnauthorized);
                return;
            }
            if (code == 200 && onBody != null) {
                String body = read(c.getInputStream());
                main.post(() -> onBody.accept(body));
            }
        } catch (IOException e) {
            Status.server(-1); // offline: the next fix or report tries again; the server lets old data expire
        } finally {
            if (c != null) c.disconnect();
        }
    }

    private static String read(InputStream in) throws IOException {
        ByteArrayOutputStream b = new ByteArrayOutputStream();
        byte[] buf = new byte[1024];
        for (int n; (n = in.read(buf)) > 0; ) b.write(buf, 0, n);
        return b.toString("UTF-8");
    }
}
