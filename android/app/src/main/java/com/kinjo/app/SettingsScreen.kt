package com.kinjo.app

import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch


@Composable
fun SettingsScreen(
    visible: Boolean,
    onVisibleChange: (Boolean) -> Unit,
    onBack: () -> Unit,
    onViewCard: () -> Unit,
    onEditProfile: () -> Unit,
    onDeleted: () -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val me = remember { ProfileStore.load(context) }
    val photo = remember { ProfileStore.photo(context) }
    var busy by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }

    fun act(action: suspend () -> Unit) {
        busy = true
        scope.launch {
            tryServer(action)?.let(::showToast)
            busy = false
        }
    }


    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize().padding(horizontal = 22.dp)) {
            Row(Modifier.fillMaxWidth().heightIn(min = 56.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(Icon.Back, "Back", onBack, Modifier.offset(x = (-12).dp))
            }
            Column(Modifier.weight(1f).verticalScroll(rememberScrollState())) {
                BasicText("Settings", style = Kinjo.title, modifier = Modifier.padding(top = 14.dp, bottom = 22.dp))

                Divider()
                Row(
                    Modifier.fillMaxWidth().clickable(role = Role.Button, onClickLabel = "View my card", onClick = onViewCard).padding(vertical = 18.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    Avatar(photo, me?.name.orEmpty(), 64.dp)
                    Column {
                        BasicText(me?.name.orEmpty(), style = Kinjo.body.copy(fontSize = 19.sp, fontWeight = FontWeight.Medium, letterSpacing = (-0.02).em))
                        BasicText(me?.role.orEmpty(), style = Kinjo.body.copy(fontSize = 14.sp, color = Kinjo.Muted))
                    }
                }
                Divider()

                Group("Profile") {
                    SettingRow(
                        Modifier.toggleable(value = visible, enabled = !busy, role = Role.Switch) { target ->
                            act {
                                Api.setVisible(context, target)
                                onVisibleChange(target)
                            }
                        },
                    ) {
                        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                            BasicText("Show my profile", style = Kinjo.body)
                            BasicText(
                                if (visible) "People within 30 m can see your card." else "Nobody can see you right now.",
                                style = Kinjo.small.copy(color = Kinjo.Muted),
                            )
                        }
                        KSwitch(visible)
                    }
                    SettingRow(Modifier.clickable(role = Role.Button, onClick = onViewCard)) { BasicText("View my card", style = Kinjo.body) }
                    SettingRow(Modifier.clickable(role = Role.Button, onClick = onEditProfile)) { BasicText("Edit profile", style = Kinjo.body) }
                }
                Group("Legal") {
                    SettingRow(Modifier.clickable(role = Role.Button) { openUrl(context, TERMS_URL) }) { BasicText("Terms & Conditions", style = Kinjo.body) }
                    SettingRow(Modifier.clickable(role = Role.Button) { openUrl(context, PRIVACY_URL) }) { BasicText("Privacy Policy", style = Kinjo.body) }
                }
                Group("Account") {
                    SettingRow(Modifier.clickable(role = Role.Button) { confirmDelete = true }) {
                        BasicText("Delete account", style = Kinjo.body.copy(color = Kinjo.Danger))
                    }
                }
                BasicText(
                    "Kinjo · version ${BuildConfig.VERSION_NAME}",
                    style = Kinjo.small.copy(fontSize = 12.sp, letterSpacing = 0.06.em),
                    modifier = Modifier.padding(top = 26.dp, bottom = 8.dp),
                )
            }
        }
        if (confirmDelete) {
            Sheet(onDismiss = { if (!busy) confirmDelete = false }) {
                Row(Modifier.fillMaxWidth().padding(bottom = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Eyebrow("Delete account", color = Kinjo.Danger)
                    Box(Modifier.weight(1f))
                    IconButton(Icon.Close, "Close", { if (!busy) confirmDelete = false }, Modifier.offset(x = 12.dp))
                }
                BasicText("Delete your Kinjo account?", style = Kinjo.cardTitle.copy(letterSpacing = (-0.045).em, lineHeight = 1.08.em))
                BasicText(
                    "Your card, photo and details are removed for good and nobody nearby can see you. This can’t be undone.",
                    style = Kinjo.body.copy(fontSize = 14.5.sp, color = Kinjo.Muted),
                    modifier = Modifier.padding(top = 10.dp),
                )
                KButton(
                    if (busy) "Deleting…" else "Delete account",
                    onClick = {
                        act {
                            Api.deleteAccount(context)
                            PresenceService.stop(context)
                            ProfileStore.clear(context)
                            nearbyCards.value = emptyList()
                            onDeleted()
                        }
                    },
                    enabled = !busy,
                    kind = ButtonKind.Danger,
                    modifier = Modifier.padding(top = 22.dp),
                )
                KButton("Keep my account", { confirmDelete = false }, enabled = !busy, kind = ButtonKind.Line, modifier = Modifier.padding(top = 10.dp))
            }
        }
    }
}

@Composable
private fun Group(label: String, rows: @Composable () -> Unit) {
    Column(Modifier.padding(top = 26.dp)) {
        BasicText(label.uppercase(), style = Kinjo.eyebrow.copy(color = Kinjo.Faint), modifier = Modifier.padding(bottom = 8.dp))
        Divider()
        rows()
    }
}

@Composable
private fun SettingRow(modifier: Modifier, content: @Composable RowScope.() -> Unit) {
    Row(
        Modifier.fillMaxWidth().heightIn(min = 58.dp).then(modifier).padding(vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        content = content,
    )
    Divider()
}

/** The old app's switch: white pill when on. The row around it handles the tap. */
@Composable
fun KSwitch(on: Boolean) {
    val knob by animateDpAsState(if (on) 20.dp else 0.dp, label = "switch")
    Box(
        Modifier
            .size(width = 50.dp, height = 30.dp)
            .clip(CircleShape)
            .background(if (on) Kinjo.Fg else Kinjo.Surface2)
            .border(1.dp, if (on) Kinjo.Fg else Kinjo.Hairline, CircleShape)
            .padding(3.dp),
    ) {
        Box(Modifier.offset { IntOffset(knob.roundToPx(), 0) }.size(24.dp).clip(CircleShape).background(if (on) Kinjo.Bg else Color.White))
    }
}
