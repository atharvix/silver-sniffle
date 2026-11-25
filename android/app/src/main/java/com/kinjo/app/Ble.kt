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

    /** (Re)starts advertising and scanning. `fast` trades battery for detection speed while moving. */
    // shortcut: Bluetooth switched on later is picked up at the next token (15 min) or mode change.
    @SuppressLint("MissingPermission") // allowed() checks it
    fun start(token: String, fast: Boolean) {
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
        heard.clear()
    }

    /** Tokens heard since the last call, strongest signal each. */
    fun drain(): Map<String, Int> = heard.toMap().also { heard.clear() }

    private companion object {
        val SERVICE: ParcelUuid = ParcelUuid(UUID.fromString("6b1f3c52-9a0e-4d7b-8f2a-4c1e9d0b7a31"))
        const val TOKEN_BYTES = 8

        fun hexToBytes(hex: String) = ByteArray(hex.length / 2) { hex.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }
}
