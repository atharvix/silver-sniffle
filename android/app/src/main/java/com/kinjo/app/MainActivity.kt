package com.kinjo.app

import android.Manifest
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
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
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        super.onCreate(savedInstanceState)
        setContent { App(this) }
    }
}

private enum class Screen { Profile, Scan, Nearby, Settings }

private fun foregroundPermissions() = buildList {
    add(Manifest.permission.ACCESS_FINE_LOCATION)
    add(Manifest.permission.ACCESS_COARSE_LOCATION)
    add(Manifest.permission.ACTIVITY_RECOGNITION) // restart when the person starts moving
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
    var canLocate by remember { mutableStateOf(context.granted(Manifest.permission.ACCESS_FINE_LOCATION)) }
    var bleAllowed by remember { mutableStateOf(Ble(context).allowed()) }
    var visible by remember { mutableStateOf(Api.visible(context)) }
    var locationRefused by rememberSaveable { mutableStateOf(false) }
    var menuOpen by rememberSaveable { mutableStateOf(false) }
    var cardOpen by rememberSaveable { mutableStateOf(false) }
    var splashing by rememberSaveable { mutableStateOf(true) }
    var keepOn by remember { mutableStateOf(KeepOn.next(context)) }
    var alwaysRefused by rememberSaveable { mutableStateOf(false) }

    // Permissions and visibility can change while we're away (system settings, another phone hiding us).
    DisposableEffect(activity) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                canLocate = context.granted(Manifest.permission.ACCESS_FINE_LOCATION)
                bleAllowed = Ble(context).allowed()
                visible = Api.visible(context)
                keepOn = KeepOn.next(context)
                Health.report(context)
            }
        }
        activity.lifecycle.addObserver(observer)
        onDispose { activity.lifecycle.removeObserver(observer) }
    }

    val askBackground = rememberLauncherForActivityResult(RequestPermission()) { granted ->
        alwaysRefused = !granted // Android may not ask again: the next tap opens the app's settings
        keepOn = KeepOn.next(context)
    }
    val askBattery = rememberLauncherForActivityResult(StartActivityForResult()) { keepOn = KeepOn.next(context) }
    // Android 11+ only accepts background location as a second, separate request.
    val askForeground = rememberLauncherForActivityResult(RequestMultiplePermissions()) {
        canLocate = context.granted(Manifest.permission.ACCESS_FINE_LOCATION)
        bleAllowed = Ble(context).allowed()
        locationRefused = !canLocate
        if (canLocate && Api.visible(context)) PresenceService.start(context, "user") // picks up a new Bluetooth grant
        if (canLocate && !context.granted(Manifest.permission.ACCESS_BACKGROUND_LOCATION)) {
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

    // Phones that granted location before a permission was added (Bluetooth, movement) are asked once more.
    LaunchedEffect(screen, canLocate) {
        val prefs = context.getSharedPreferences("app", Context.MODE_PRIVATE)
        val firstAsk = setOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION, "android.permission.POST_NOTIFICATIONS")
        val missing = foregroundPermissions().any { it !in firstAsk && !context.granted(it) }
        if (screen == Screen.Nearby && canLocate && missing && !prefs.getBoolean("askedAddedPermissions", false)) {
            prefs.edit().putBoolean("askedAddedPermissions", true).apply()
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
        val mayAsk = Build.VERSION.SDK_INT < 31 || context.granted(Manifest.permission.BLUETOOTH_CONNECT)
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
        // Screens cross-fade like the old app: the old one goes in 0.38 s, the new one comes in over 0.5 s.
        AnimatedContent(
            screen,
            transitionSpec = { fadeIn(tween(500, delayMillis = 50, easing = Ease)) togetherWith fadeOut(tween(380, easing = Ease)) },
            label = "screen",
        ) { shownScreen ->
            when (shownScreen) {
                Screen.Profile -> ProfileScreen(onDone = { screen = Screen.Scan })
                Screen.Scan -> ScanScreen(ready = canLocate, onDone = { screen = Screen.Nearby })
                Screen.Nearby -> NearbyScreen(
                    visible = visible,
                    onLookAgain = { screen = Screen.Scan },
                    onMenu = { menuOpen = true },
                    onShowProfile = {
                        scope.launch {
                            tryServer {
                                Api.setVisible(context, true)
                                visible = true
                            }?.let(::showToast)
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
        }
        Drawer(
            visible = menuOpen,
            onClose = { menuOpen = false },
            onViewCard = { cardOpen = true },
            onSettings = {
                menuOpen = false
                screen = Screen.Settings
            },
        )
        CardViewer(
            visible = cardOpen,
            onClose = { cardOpen = false },
            onEdit = {
                cardOpen = false
                menuOpen = false
                screen = Screen.Profile
            },
        )
        // Location can't be skipped: without it nobody can be found.
        PermissionCard(
            visible = screen != Screen.Profile && !canLocate,
            title = "See who’s around you",
            body = "Grant Kinjo permission to access your device location while you’re using the app. " +
                "This helps us show you profiles within 30 m that you could connect with.",
            button = if (locationRefused) "Open settings" else "Allow",
            onClick = {
                // After a refusal Android may not ask again: send people to the app's settings instead.
                if (locationRefused) {
                    context.startActivity(KeepOn.appSettings(context))
                } else {
                    askForeground.launch(foregroundPermissions())
                }
            },
        )
        // Until the phone lets Kinjo run in the background, swiping it away or a sleeping phone can hide you.
        // Not dismissable: this is what keeps people visible, one step at a time.
        val step = keepOn
        PermissionCard(
            visible = screen == Screen.Nearby && canLocate && visible && step != null,
            title = "Keep Kinjo on",
            body = when (step) {
                KeepOn.Step.Location -> "Set location to “Allow all the time”, so people around you can still find you " +
                    "when Kinjo is closed or your screen is off."
                KeepOn.Step.Battery -> "Let Kinjo run in the background. Otherwise your phone stops it to save battery " +
                    "and you disappear for people around you."
                else -> "Turn on Auto-start for Kinjo. This phone clears apps it doesn’t know, and you’d disappear until " +
                    "you open Kinjo again."
            },
            button = if (step == KeepOn.Step.Battery) "Allow" else "Open settings",
            onClick = {
                when (step) {
                    KeepOn.Step.Location ->
                        if (alwaysRefused) context.startActivity(KeepOn.appSettings(context))
                        else askBackground.launch(Manifest.permission.ACCESS_BACKGROUND_LOCATION)
                    KeepOn.Step.Battery ->
                        runCatching { askBattery.launch(KeepOn.batteryRequest(context)) }
                            .onFailure { context.startActivity(KeepOn.appSettings(context)) }
                    else -> {
                        KeepOn.openAutoStart(context)
                        keepOn = KeepOn.next(context)
                    }
                }
            },
        )
        ToastHost(Modifier.align(Alignment.BottomCenter))
    }
    // The old splash plays once per launch, over the app loading underneath (outside the insets: full screen).
    if (splashing) Splash(onDone = { splashing = false })
}
