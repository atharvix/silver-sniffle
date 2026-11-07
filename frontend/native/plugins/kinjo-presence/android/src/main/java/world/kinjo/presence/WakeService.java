package world.kinjo.presence;

import androidx.annotation.NonNull;
import com.capacitorjs.plugins.pushnotifications.MessagingService;
import com.google.firebase.messaging.RemoteMessage;

/**
 * Receives push messages instead of Capacitor's service (the manifest removes Capacitor's and
 * registers this one; drop both together or pushes stop arriving) and handles the server's
 * silent "wake" itself. Every other message goes to Capacitor as before.
 */
public class WakeService extends MessagingService {
    @Override
    public void onMessageReceived(@NonNull RemoteMessage m) {
        if ("wake".equals(m.getData().get("kind"))) {
            // a running service just scans now, at any priority; only a high-priority push may start one
            PresenceService.poke(this, m.getPriority() == RemoteMessage.PRIORITY_HIGH);
            return;
        }
        super.onMessageReceived(m);
    }
}
