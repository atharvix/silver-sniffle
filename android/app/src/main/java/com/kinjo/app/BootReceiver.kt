package com.kinjo.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/** Boot and app updates are official exemptions for starting a foreground service from the background. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED -> PresenceService.start(context, "boot")
            Intent.ACTION_MY_PACKAGE_REPLACED -> PresenceService.start(context, "updated")
        }
    }
}
