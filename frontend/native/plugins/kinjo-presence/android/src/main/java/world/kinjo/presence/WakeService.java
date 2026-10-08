package world.kinjo.presence;

import androidx.annotation.NonNull;
import com.capacitorjs.plugins.pushnotifications.MessagingService;
import com.google.firebase.messaging.RemoteMessage;

/**
 * Receives push messages instead of Capacitor's service (higher priority in the manifest)
 * and handles the server's silent "wake" push itself: it restarts the location service on
 * a phone that went quiet. Every other message goes to Capacitor as before.
 */
public class WakeService extends MessagingService {
    @Override
    public void onMessageReceived(@NonNull RemoteMessage m) {
        if ("wake".equals(m.getData().get("kind"))) {
            PresenceService.revive(this);
            return;
        }
        super.onMessageReceived(m);
    }
}
