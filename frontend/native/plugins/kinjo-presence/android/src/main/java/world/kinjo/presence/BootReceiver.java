package world.kinjo.presence;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** After a reboot or an app update, resume sharing for a signed-in user, without opening the app. */
public class BootReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context c, Intent i) {
        // Starting location tracking from the background needs "Allow all the time".
        if (PresenceService.signedIn(c) && PresencePlugin.hasBackground(c)) PresenceService.start(c);
    }
}
