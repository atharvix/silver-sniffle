package com.kinjo.app

import android.Manifest
import android.app.ActivityManager
import android.app.ApplicationExitInfo
import android.app.usage.UsageStatsManager
import android.content.Context
import android.os.Build
import android.os.PowerManager
import androidx.core.app.NotificationManagerCompat
import org.json.JSONObject
import java.util.UUID

/**
 * What keeps Kinjo alive in the background on this phone, or stops it. Sent to the server for the admin
 * dashboard, at most once an hour. Settings and counts only: no position, no list of installed apps.
 */
object Health {
    // Cleaner and booster apps known to stop other apps. Android only reveals installed apps that the manifest
    // names in <queries>, so this checks exactly these and nothing else (keep the two lists in sync).
    private val KILLERS = mapOf(
        "com.cleanmaster.mguard" to "Clean Master",
        "com.piriform.ccleaner" to "CCleaner",
        "com.avast.android.cleaner" to "Avast Cleanup",
        "com.avg.cleaner" to "AVG Cleaner",
        "com.oasisfeng.greenify" to "Greenify",
        "com.transsion.phonemaster" to "Phone Master",
    )

    private const val EVERY_MS = 60 * 60 * 1000L

    /** Sends a snapshot in the background, at most once an hour, once the card is on the server. */
    fun report(context: Context) {
        val app = context.applicationContext
        val prefs = app.getSharedPreferences("health", Context.MODE_PRIVATE)
        val now = System.currentTimeMillis()
        if (Api.token(app) == null || now - prefs.getLong("sentAt", 0) < EVERY_MS) return
        prefs.edit().putLong("sentAt", now).apply()
        Api.sendHealth(app, snapshot(app)) { prefs.edit().remove("sentAt").apply() } // failed: retry next time
    }

    private fun snapshot(context: Context): JSONObject {
        val power = context.getSystemService(PowerManager::class.java)
        val prefs = context.getSharedPreferences("health", Context.MODE_PRIVATE)
        val installId = prefs.getString("installId", null) ?: UUID.randomUUID().toString().also {
            prefs.edit().putString("installId", it).apply()
        }
        return JSONObject()
            .put("installId", installId)
            .put("platform", "android")
            .put("maker", Build.MANUFACTURER.take(40))
            .put("model", Build.MODEL.take(60))
            .put("os", "Android ${Build.VERSION.RELEASE}".take(30))
            .put("app", BuildConfig.VERSION_NAME.take(20))
            .put("batteryFree", power.isIgnoringBatteryOptimizations(context.packageName))
            .put("bgRestricted", context.getSystemService(ActivityManager::class.java).isBackgroundRestricted)
            .put("standbyBucket", context.getSystemService(UsageStatsManager::class.java).appStandbyBucket)
            .put("powerSave", power.isPowerSaveMode)
            .put("locationAlways", context.granted(Manifest.permission.ACCESS_BACKGROUND_LOCATION))
            .put("bluetoothOn", Ble(context).isOn())
            .put("notificationsOn", NotificationManagerCompat.from(context).areNotificationsEnabled())
            .put("lastExit", lastExit(context))
            .put("killerApps", KILLERS.filterKeys { installed(context, it) }.values.joinToString(","))
    }

    private fun installed(context: Context, pkg: String) =
        runCatching { context.packageManager.getPackageInfo(pkg, 0) }.isSuccess

    /** Why Android last stopped Kinjo's process (Android 11+). A swipe alone doesn't stop it. */
    private fun lastExit(context: Context): String {
        if (Build.VERSION.SDK_INT < 30) return "unknown (Android 10)"
        val info = context.getSystemService(ActivityManager::class.java)
            .getHistoricalProcessExitReasons(context.packageName, 0, 1).firstOrNull() ?: return "none"
        return when (info.reason) {
            ApplicationExitInfo.REASON_USER_REQUESTED -> "force stop"
            ApplicationExitInfo.REASON_LOW_MEMORY -> "low memory"
            ApplicationExitInfo.REASON_SIGNALED -> "killed by the system"
            ApplicationExitInfo.REASON_OTHER -> "other (often the phone maker)"
            ApplicationExitInfo.REASON_EXCESSIVE_RESOURCE_USAGE -> "too much battery use"
            ApplicationExitInfo.REASON_CRASH, ApplicationExitInfo.REASON_CRASH_NATIVE -> "crash"
            ApplicationExitInfo.REASON_ANR -> "not responding"
            ApplicationExitInfo.REASON_EXIT_SELF -> "closed itself"
            ApplicationExitInfo.REASON_PERMISSION_CHANGE -> "permission changed"
            ApplicationExitInfo.REASON_DEPENDENCY_DIED -> "dependency died"
            ApplicationExitInfo.REASON_INITIALIZATION_FAILURE -> "failed to start"
            else -> if (Build.VERSION.SDK_INT >= 34 && info.reason == ApplicationExitInfo.REASON_PACKAGE_UPDATED) "app updated"
            else if (Build.VERSION.SDK_INT >= 34 && info.reason == ApplicationExitInfo.REASON_FREEZER) "frozen by the system"
            else "unknown"
        }
    }
}
