package com.kinjo.app

import android.Manifest
import android.annotation.SuppressLint
import android.bluetooth.BluetoothManager
import android.bluetooth.le.AdvertiseCallback
import android.bluetooth.le.AdvertiseData
import android.bluetooth.le.AdvertiseSettings
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.os.ParcelUuid
import java.util.UUID

/**
 * Advertises this phone's server-issued token and hears other phones' tokens.
 * The token is random and rotates every 15 minutes, so it identifies nobody to anyone but our server.
 * All calls and callbacks run on the main thread.
 */
class Ble(private val context: Context) {

    private val adapter get() = context.getSystemService(BluetoothManager::class.java)?.adapter
    private var advertising: AdvertiseCallback? = null
    private var scanning: ScanCallback? = null
    private val heard = mutableMapOf<String, Int>() // token -> strongest RSSI since the last drain()

    fun allowed() = Build.VERSION.SDK_INT < 31 ||
        listOf(Manifest.permission.BLUETOOTH_SCAN, Manifest.permission.BLUETOOTH_ADVERTISE)
            .all { context.checkSelfPermission(it) == PackageManager.PERMISSION_GRANTED }

    private var current: Pair<String, Boolean>? = null // what is running now: token, fast

    /**
     * Starts advertising and scanning, or switches to a new token or speed. `fast` trades battery for detection
     * speed while moving. Calling it again with the same values does nothing: Android silently blocks an app that
     * starts more than 5 scans in 30 seconds.
     */
    @SuppressLint("MissingPermission") // allowed() checks it
    fun start(token: String, fast: Boolean) {
        if (current == token to fast && isOn()) return
        stop()
        val bt = adapter
        if (!allowed() || bt == null || !bt.isEnabled) return trail(context, "ble off")

        val advertiser = bt.bluetoothLeAdvertiser
        val scanner = bt.bluetoothLeScanner
        if (advertiser == null || scanner == null) return trail(context, "ble unsupported")

        val adSettings = AdvertiseSettings.Builder()
            .setAdvertiseMode(if (fast) AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY else AdvertiseSettings.ADVERTISE_MODE_LOW_POWER)
            .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_MEDIUM)
            .setConnectable(false)
            .build()
        // Service data alone (no separate UUID list) keeps the packet within the 31-byte limit.
        val adData = AdvertiseData.Builder().addServiceData(SERVICE, hexToBytes(token)).build()
        advertising = object : AdvertiseCallback() {
            override fun onStartFailure(errorCode: Int) = trail(context, "ble advertise failed $errorCode")
        }.also { advertiser.startAdvertising(adSettings, adData, it) }

        val filter = ScanFilter.Builder().setServiceData(SERVICE, ByteArray(0)).build()
        val scanSettings = ScanSettings.Builder()
            .setScanMode(if (fast) ScanSettings.SCAN_MODE_LOW_LATENCY else ScanSettings.SCAN_MODE_LOW_POWER)
            .build()
        scanning = object : ScanCallback() {
            override fun onScanResult(callbackType: Int, result: ScanResult) {
                val data = result.scanRecord?.getServiceData(SERVICE) ?: return
                if (data.size != TOKEN_BYTES) return
                val token = data.joinToString("") { "%02x".format(it) }
                heard[token] = maxOf(heard[token] ?: Int.MIN_VALUE, result.rssi)
            }

            override fun onScanFailed(errorCode: Int) = trail(context, "ble scan failed $errorCode")
        }.also { scanner.startScan(listOf(filter), scanSettings, it) }
        current = token to fast
        trail(context, "ble on fast=$fast")
    }

    @SuppressLint("MissingPermission") // only stops what start() began with the permission
    fun stop() {
        val bt = adapter
        if (bt != null && allowed() && bt.isEnabled) {
            advertising?.let { bt.bluetoothLeAdvertiser?.stopAdvertising(it) }
            scanning?.let { bt.bluetoothLeScanner?.stopScan(it) }
        }
        advertising = null
        scanning = null
        current = null
        heard.clear()
    }

    fun isOn() = adapter?.isEnabled == true

    /**
     * Switches Bluetooth on without asking, where Android still lets an app do that (Android 12 and older).
     * On 13+ it returns false and the open app shows Android's one-tap "turn on Bluetooth" prompt instead.
     */
    @Suppress("DEPRECATION") // enable() is the only silent way on Android 12 and older; it's a no-op on 13+
    @SuppressLint("MissingPermission") // checked just above the call
    fun switchOn(): Boolean {
        val bt = adapter ?: return false
        if (bt.isEnabled) return true
        val mayConnect = Build.VERSION.SDK_INT < 31 ||
            context.checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED
        if (Build.VERSION.SDK_INT >= 33 || !mayConnect) return false
        return bt.enable()
    }

    /** Tokens heard since the last call, strongest signal each. */
    fun drain(): Map<String, Int> = heard.toMap().also { heard.clear() }

    private companion object {
        val SERVICE: ParcelUuid = ParcelUuid(UUID.fromString("6b1f3c52-9a0e-4d7b-8f2a-4c1e9d0b7a31"))
        const val TOKEN_BYTES = 8

        fun hexToBytes(hex: String) = ByteArray(hex.length / 2) { hex.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }
}
