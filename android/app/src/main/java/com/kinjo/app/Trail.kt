package com.kinjo.app

import android.content.Context
import java.io.File
import java.time.Instant

// Read on a test phone with: adb shell run-as com.kinjo.app cat files/trail.log (older lines: files/trail.old.log)
private val lock = Any()
private const val MAX_BYTES = 2_000_000L // ~10 hours while moving; Kinjo runs for months, so the log rotates

fun trail(context: Context, line: String) = synchronized(lock) {
    val file = File(context.filesDir, "trail.log")
    if (file.length() > MAX_BYTES) file.renameTo(File(context.filesDir, "trail.old.log"))
    file.appendText("${Instant.now()} $line\n")
}
