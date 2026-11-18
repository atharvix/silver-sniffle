package world.kinjo.presence;

import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.provider.Settings;
import androidx.core.app.NotificationCompat;

/**
 * The two times Kinjo has to tell someone they've gone invisible (docs/proximity.md):
 * after a restart (Android won't start location from the background, so one tap brings it
 * back) and while location is switched off. Each goes away once fixed. Nothing shows without
 * notification permission. Placeholder copy until the founder writes it.
 */
final class Notify {
    static final String CHANNEL = "kinjo_status";
    static final int RESTART = 31, LOCATION_OFF = 32;

    static void stayVisible(Context c) {
        Intent open = c.getPackageManager().getLaunchIntentForPackage(c.getPackageName());
        show(c, RESTART, "Tap to stay visible", "Your phone restarted. Open Kinjo so people near you can see you again.", open);
    }

    static void locationOff(Context c, boolean off) {
        if (!off) { cancel(c, LOCATION_OFF); return; }
        Intent settings = new Intent(Settings.ACTION_LOCATION_SOURCE_SETTINGS);
        show(c, LOCATION_OFF, "Location is off", "People near you can't see you, and you can't see them. Tap to turn it on.", settings);
    }

    static void cancel(Context c, int id) {
        NotificationManager nm = (NotificationManager) c.getSystemService(Context.NOTIFICATION_SERVICE);
        if (nm != null) nm.cancel(id);
    }

    private static void show(Context c, int id, String title, String text, Intent tap) {
        NotificationManager nm = (NotificationManager) c.getSystemService(Context.NOTIFICATION_SERVICE);
        if (nm == null || tap == null) return;
        if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(new NotificationChannel(CHANNEL, "Visibility", NotificationManager.IMPORTANCE_DEFAULT));
        }
        tap.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        PendingIntent pi = PendingIntent.getActivity(c, id, tap, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        try {
            nm.notify(id, new NotificationCompat.Builder(c, CHANNEL)
                .setSmallIcon(R.drawable.ic_stat_kinjo)
                .setContentTitle(title)
                .setContentText(text)
                .setStyle(new NotificationCompat.BigTextStyle().bigText(text))
                .setAutoCancel(true)
                .setContentIntent(pi)
                .build());
        } catch (SecurityException e) {
            // notifications not allowed: nothing to show
        }
    }
}
