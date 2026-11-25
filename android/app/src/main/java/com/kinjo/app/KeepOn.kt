package com.kinjo.app

import android.Manifest
import android.annotation.SuppressLint
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings

/**
 * What a phone needs so Kinjo keeps running after it's swiped away, the screen is off, or the phone clears apps.
 * Asked one step at a time until all are done.
 */
object KeepOn {
    enum class Step { Location, Battery, AutoStart }

    fun next(context: Context): Step? = when {
        !context.granted(Manifest.permission.ACCESS_BACKGROUND_LOCATION) -> Step.Location
        !context.getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(context.packageName) -> Step.Battery
        autoStartScreens.isNotEmpty() && !prefs(context).getBoolean("autoStartOpened", false) -> Step.AutoStart
        else -> null
    }

    /** Android's own "Allow Kinjo to always run in the background?" dialog. Kinjo can't find anyone otherwise. */
    @SuppressLint("BatteryLife")
    fun batteryRequest(context: Context) =
        Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, Uri.fromParts("package", context.packageName, null))

    fun appSettings(context: Context) =
        Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))

    /** Opens this maker's auto-start screen, or the app's settings if none of the known screens opens. */
    fun openAutoStart(context: Context) {
        prefs(context).edit().putBoolean("autoStartOpened", true).apply()
        val opened = autoStartScreens.any { (pkg, cls) ->
            runCatching { context.startActivity(Intent().setComponent(ComponentName(pkg, cls)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }.isSuccess
        }
        if (!opened) context.startActivity(appSettings(context))
    }

    // Makers that block apps from restarting unless "Auto-start" is on. Samsung and Pixel follow the battery setting.
    private val autoStartScreens: List<Pair<String, String>> = when (Build.MANUFACTURER.lowercase()) {
        "xiaomi", "redmi", "poco" -> listOf("com.miui.securitycenter" to "com.miui.permcenter.autostart.AutoStartManagementActivity")
        "oppo", "realme", "oneplus" -> listOf(
            "com.coloros.safecenter" to "com.coloros.safecenter.permission.startup.StartupAppListActivity",
            "com.coloros.safecenter" to "com.coloros.safecenter.startupapp.StartupAppListActivity",
            "com.oplus.safecenter" to "com.oplus.safecenter.permission.startup.StartupAppListActivity",
        )
        "vivo", "iqoo" -> listOf(
            "com.vivo.permissionmanager" to "com.vivo.permissionmanager.activity.BgStartUpManagerActivity",
            "com.iqoo.secure" to "com.iqoo.secure.ui.phoneoptimize.BgStartUpManager",
        )
        "huawei" -> listOf("com.huawei.systemmanager" to "com.huawei.systemmanager.startupmgr.ui.StartupNormalAppListActivity")
        "honor" -> listOf("com.hihonor.systemmanager" to "com.hihonor.systemmanager.startupmgr.ui.StartupNormalAppListActivity")
        else -> emptyList()
    }

    private fun prefs(context: Context) = context.getSharedPreferences("app", Context.MODE_PRIVATE)
}
