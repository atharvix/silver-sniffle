package world.kinjo.presence;

import com.getcapacitor.JSObject;

/** What the service is doing right now, for the app's "Location check" row. In memory only. */
final class Status {
    static volatile boolean running, bleOn;
    static volatile long fixAt, serverAt; // wall-clock ms of the last fix / last answer from the server
    static volatile float fixAcc;
    static volatile int serverCode, bleHeard; // last HTTP status (-1 = no connection); phones heard in the last 2 min

    static void server(int code) {
        serverCode = code;
        serverAt = System.currentTimeMillis();
    }

    static JSObject json() {
        long now = System.currentTimeMillis();
        JSObject o = new JSObject();
        o.put("running", running);
        o.put("fixAge", fixAt == 0 ? -1 : (now - fixAt) / 1000);
        o.put("fixAcc", Math.round(fixAcc));
        o.put("bluetooth", bleOn);
        o.put("heard", bleHeard);
        o.put("server", serverCode);
        o.put("serverAge", serverAt == 0 ? -1 : (now - serverAt) / 1000);
        return o;
    }
}
