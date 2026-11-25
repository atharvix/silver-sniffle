package com.kinjo.app

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts.RequestMultiplePermissions
import androidx.activity.result.contract.ActivityResultContracts.RequestPermission
import androidx.activity.result.contract.ActivityResultContracts.StartActivityForResult
import android.bluetooth.BluetoothAdapter
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import kotlinx.coroutines.launch
import java.io.IOException

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        super.onCreate(savedInstanceState)
        setContent { App(this) }
    }
}

private enum class Screen { Profile, Scan, Nearby, Settings }

private fun granted(context: Context, permission: String) =
    ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED

private fun foregroundPermissions() = buildList {
    add(Manifest.permission.ACCESS_FINE_LOCATION)
    add(Manifest.permission.ACCESS_COARSE_LOCATION)
    if (Build.VERSION.SDK_INT >= 31) {
        add(Manifest.permission.BLUETOOTH_SCAN)
        add(Manifest.permission.BLUETOOTH_ADVERTISE)
        add(Manifest.permission.BLUETOOTH_CONNECT)
    }
    if (Build.VERSION.SDK_INT >= 33) add(Manifest.permission.POST_NOTIFICATIONS)
}.toTypedArray()

@Composable
private fun App(activity: ComponentActivity) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var screen by rememberSaveable {
        // No server session means the card was never uploaded: finish it on the profile screen first.
        mutableStateOf(if (ProfileStore.load(context) == null || Api.token(context) == null) Screen.Profile else Screen.Scan)
    }
    var canLocate by remember { mutableStateOf(granted(context, Manifest.permission.ACCESS_FINE_LOCATION)) }
    var bleAllowed by remember { mutableStateOf(Ble(context).allowed()) }
    var visible by remember { mutableStateOf(Api.visible(context)) }
    var locationRefused by rememberSaveable { mutableStateOf(false) }
    var menuOpen by rememberSaveable { mutableStateOf(false) }
    var cardOpen by rememberSaveable { mutableStateOf(false) }

    // Permissions and visibility can change while we're away (system settings, another phone hiding us).
    DisposableEffect(activity) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                canLocate = granted(context, Manifest.permission.ACCESS_FINE_LOCATION)
                bleAllowed = Ble(context).allowed()
                visible = Api.visible(context)
            }
        }
        activity.lifecycle.addObserver(observer)
        onDispose { activity.lifecycle.removeObserver(observer) }
    }

    val askBackground = rememberLauncherForActivityResult(RequestPermission()) { }
    // Android 11+ only accepts background location as a second, separate request.
    val askForeground = rememberLauncherForActivityResult(RequestMultiplePermissions()) {
        canLocate = granted(context, Manifest.permission.ACCESS_FINE_LOCATION)
        bleAllowed = Ble(context).allowed()
        locationRefused = !canLocate
        if (canLocate && Api.visible(context)) PresenceService.start(context, "user") // picks up a new Bluetooth grant
        if (canLocate && !granted(context, Manifest.permission.ACCESS_BACKGROUND_LOCATION)) {
            askBackground.launch(Manifest.permission.ACCESS_BACKGROUND_LOCATION)
        }
    }

    LaunchedEffect(screen, canLocate, visible) {
        if (screen != Screen.Profile && canLocate && visible) PresenceService.start(context, "user")
    }

    // Once the card is on the server, the server may wake this phone: send it the push token.
    LaunchedEffect(screen == Screen.Profile) {
        if (screen != Screen.Profile && Api.token(context) != null) Api.syncPushToken(context)
    }

    // Phones that granted location before Bluetooth was added are asked for it once.
    LaunchedEffect(screen, canLocate) {
        val prefs = context.getSharedPreferences("app", Context.MODE_PRIVATE)
        val missing = foregroundPermissions().any { it.startsWith("android.permission.BLUETOOTH") && !granted(context, it) }
        if (screen == Screen.Nearby && canLocate && missing && !prefs.getBoolean("askedBluetooth", false)) {
            prefs.edit().putBoolean("askedBluetooth", true).apply()
            askForeground.launch(foregroundPermissions())
        }
    }

    // Bluetooth off on the nearby screen: switch it on, or (Android 13+) show Android's one-tap prompt, once per app open.
    var promptedBluetoothOn by rememberSaveable { mutableStateOf(false) }
    val askBluetoothOn = rememberLauncherForActivityResult(StartActivityForResult()) {
        if (Api.visible(context)) PresenceService.start(context, "user")
    }
    LaunchedEffect(screen, canLocate, bleAllowed) {
        val ble = Ble(context)
        if (screen != Screen.Nearby || !canLocate || !bleAllowed || promptedBluetoothOn || ble.isOn() || ble.switchOn()) return@LaunchedEffect
        promptedBluetoothOn = true
        val mayAsk = Build.VERSION.SDK_INT < 31 || granted(context, Manifest.permission.BLUETOOTH_CONNECT)
        if (mayAsk) runCatching { askBluetoothOn.launch(Intent(BluetoothAdapter.ACTION_REQUEST_ENABLE)) }
    }

    val hasAccount = Api.token(context) != null
    BackHandler(enabled = menuOpen || cardOpen || screen == Screen.Settings || (screen == Screen.Profile && hasAccount)) {
        when {
            cardOpen -> cardOpen = false
            menuOpen -> menuOpen = false
            else -> screen = Screen.Nearby // Settings, or leaving an edit without saving
        }
    }

    Box(Modifier.fillMaxSize().background(Kinjo.Bg).windowInsetsPadding(WindowInsets.safeDrawing)) {
        when (screen) {
            Screen.Profile -> ProfileScreen(onDone = { screen = Screen.Scan })
            Screen.Scan -> ScanScreen(ready = canLocate, onDone = { screen = Screen.Nearby })
            Screen.Nearby -> NearbyScreen(
                visible = visible,
                onLookAgain = { screen = Screen.Scan },
                onMenu = { menuOpen = true },
                onShowProfile = {
                    scope.launch {
                        try {
                            Api.setVisible(context, true)
                            visible = true
                        } catch (e: ApiError) {
                            showToast(e.message ?: OFFLINE)
                        } catch (e: IOException) {
                            showToast(OFFLINE)
                        }
                    }
                },
            )
            Screen.Settings -> SettingsScreen(
                visible = visible,
                onVisibleChange = { target ->
                    visible = target
                    if (target) PresenceService.start(context, "user") else PresenceService.stop(context)
                },
                onBack = { screen = Screen.Nearby },
                onViewCard = { cardOpen = true },
                onEditProfile = { screen = Screen.Profile },
                onDeleted = { screen = Screen.Profile },
            )
        }
        if (menuOpen) {
            Drawer(
                onClose = { menuOpen = false },
                onViewCard = { cardOpen = true },
                onSettings = {
                    menuOpen = false
                    screen = Screen.Settings
                },
            )
        }
        if (cardOpen) {
            CardViewer(
                onClose = { cardOpen = false },
                onEdit = {
                    cardOpen = false
                    menuOpen = false
                    screen = Screen.Profile
                },
            )
        }
        // Location can't be skipped: without it nobody can be found.
        if (screen != Screen.Profile && !canLocate) {
            PermissionCard(
                title = "See who’s around you",
                body = "Grant Kinjo permission to access your device location while you’re using the app. " +
                    "This helps us show you profiles within 30 m that you could connect with.",
                button = if (locationRefused) "Open settings" else "Allow",
                onClick = {
                    // After a refusal Android may not ask again: send people to the app's settings instead.
                    if (locationRefused) {
                        context.startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null)))
                    } else {
                        askForeground.launch(foregroundPermissions())
                    }
                },
            )
        }
        ToastHost(Modifier.align(Alignment.BottomCenter))
    }
}
