package com.kinjo.app

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import kotlinx.coroutines.runBlocking

/**
 * Receives the server's silent wake. A high-priority FCM message is one of Android's official
 * exemptions for starting a foreground service from the background, so a swiped-away phone
 * restarts presence and reports where it is.
 */
class WakeService : FirebaseMessagingService() {

    override fun onMessageReceived(message: RemoteMessage) {
        if (message.data["type"] != "wake") return
        trail(this, "wake priority=${message.priority}")
        PresenceService.start(this, "fcm")
    }

    // Runs on a background thread, so the network call may block here.
    override fun onNewToken(token: String) {
        runCatching { runBlocking { Api.registerDevice(this@WakeService, token) } }
            .onFailure { trail(this, "push token not sent: ${it.message}") }
    }
}
