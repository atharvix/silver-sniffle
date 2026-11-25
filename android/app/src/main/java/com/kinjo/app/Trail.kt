package com.kinjo.app

import android.content.Context
import java.io.File
import java.time.Instant

// shortcut: grows ~5 MB/day while moving, fine for device tests; rotate or drop before release.
private fun trailFile(context: Context) = File(context.filesDir, "trail.log")

// Read on a test phone with: adb shell run-as com.kinjo.app.dev cat files/trail.log
private val lock = Any()

fun trail(context: Context, line: String) = synchronized(lock) { trailFile(context).appendText("${Instant.now()} $line\n") }
