package world.kinjo.presence;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import androidx.annotation.NonNull;
import androidx.work.ExistingPeriodicWorkPolicy;
import androidx.work.PeriodicWorkRequest;
import androidx.work.WorkManager;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import java.util.concurrent.TimeUnit;

/** Every 15 minutes: start the service if a phone maker's battery manager killed it. */
public class Check extends Worker {
    private static final String WORK = "kinjo-revive";

    public Check(@NonNull Context c, @NonNull WorkerParameters p) {
        super(c, p);
    }

    static void arm(Context c) {
        PeriodicWorkRequest w = new PeriodicWorkRequest.Builder(Check.class, 15, TimeUnit.MINUTES).build();
        WorkManager.getInstance(c).enqueueUniquePeriodicWork(WORK, ExistingPeriodicWorkPolicy.KEEP, w);
    }

    /** Signed out or hidden: no more restarts. */
    static void disarm(Context c) {
        WorkManager.getInstance(c).cancelUniqueWork(WORK);
    }

    @NonNull
    @Override
    public Result doWork() {
        if (!Status.running) PresenceService.revive(getApplicationContext());
        return Result.success();
    }

    /** After a reboot or an app update, resume for a signed-in user without opening the app. */
    public static class Boot extends BroadcastReceiver {
        @Override
        public void onReceive(Context c, Intent i) {
            String a = i.getAction();
            if (Intent.ACTION_BOOT_COMPLETED.equals(a) || Intent.ACTION_MY_PACKAGE_REPLACED.equals(a)) {
                PresenceService.revive(c);
                // location can't start from here (Android rule): one tap on this brings it back
                if (PresenceService.signedIn(c) && Gps.located(c)) Notify.stayVisible(c);
            }
        }
    }
}
