package com.kinjo.app

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp

/** The old app's side menu: slides in from the right, then your avatar, name and role, then Settings. */
@Composable
fun Drawer(visible: Boolean, onClose: () -> Unit, onViewCard: () -> Unit, onSettings: () -> Unit) {
    AnimatedVisibility(visible, enter = fadeIn(tween(500, easing = Ease)), exit = fadeOut(tween(500, easing = Ease))) {
        val context = LocalContext.current
        val me = remember { ProfileStore.load(context) }
        val photo = remember { ProfileStore.photo(context) }
        Box(Modifier.fillMaxSize()) {
            Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.6f)).clickable(interactionSource = null, indication = null, onClick = onClose))
            Column(
                Modifier
                    .align(Alignment.CenterEnd)
                    .animateEnterExit(
                        enter = slideInHorizontally(tween(700, easing = Ease)) { it },
                        exit = slideOutHorizontally(tween(500, easing = Ease)) { it },
                    )
                    .fillMaxHeight()
                    .fillMaxWidth(0.82f)
                    .background(Kinjo.Bg)
                    .drawBehind { drawLine(Kinjo.Hairline, Offset.Zero, Offset(0f, size.height), 1.dp.toPx()) }
                    .clickable(interactionSource = null, indication = null) {} // taps inside stay inside
                    .padding(start = 24.dp, end = 24.dp, top = 8.dp),
            ) {
                IconButton(Icon.Close, "Close menu", onClose, Modifier.align(Alignment.End).offset(x = 12.dp))
                Rise(delayMs = 180, fromSide = true) {
                    Column(Modifier.padding(top = 18.dp, bottom = 26.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                        Avatar(photo, me?.name.orEmpty(), 84.dp, Modifier.clickable(role = Role.Button, onClickLabel = "View my card", onClick = onViewCard))
                        Column {
                            BasicText(me?.name.orEmpty(), style = Kinjo.body.copy(fontSize = 26.sp, letterSpacing = (-0.04).em, lineHeight = 1.1.em))
                            BasicText(me?.role.orEmpty(), style = Kinjo.body.copy(fontSize = 14.sp, color = Kinjo.Muted), modifier = Modifier.padding(top = 4.dp))
                        }
                    }
                }
                Rise(delayMs = 260, fromSide = true) {
                    Column {
                        Divider()
                        Row(
                            Modifier.fillMaxWidth().heightIn(min = 58.dp).clickable(role = Role.Button, onClick = onSettings),
                            verticalAlignment = Alignment.CenterVertically,
                        ) { BasicText("Settings", style = Kinjo.body) }
                        Divider()
                    }
                }
            }
        }
    }
}

/** "Your card": exactly what people nearby see. The screen zooms in while the card turns straight. */
@Composable
fun CardViewer(visible: Boolean, onClose: () -> Unit, onEdit: () -> Unit) {
    AnimatedVisibility(
        visible,
        enter = fadeIn(tween(450, easing = Ease)) + scaleIn(tween(600, easing = Ease), initialScale = 0.97f),
        exit = fadeOut(tween(450, easing = Ease)),
    ) {
        val context = LocalContext.current
        val me = remember { ProfileStore.load(context) }
        val photo = remember { ProfileStore.photo(context)?.let(::BitmapPainter) }
        val settle = remember { Animatable(0f) }
        LaunchedEffect(Unit) { settle.animateTo(1f, tween(800, delayMillis = 100, easing = Ease)) }
        Column(Modifier.fillMaxSize().background(Kinjo.Bg).clickable(interactionSource = null, indication = null) {}.padding(horizontal = 22.dp)) {
            Row(Modifier.fillMaxWidth().heightIn(min = 54.dp), verticalAlignment = Alignment.CenterVertically) {
                Eyebrow("Your card")
                Box(Modifier.weight(1f))
                IconButton(Icon.Close, "Close", onClose, Modifier.offset(x = 12.dp))
            }
            Box(Modifier.weight(1f).fillMaxWidth().padding(vertical = 12.dp), contentAlignment = Alignment.Center) {
                PersonCard(
                    photo, me?.name.orEmpty(), me?.role.orEmpty(), me?.looking.orEmpty(),
                    Modifier.widthIn(max = 314.dp).graphicsLayer {
                        alpha = settle.value
                        translationY = 24.dp.toPx() * (1 - settle.value)
                        rotationZ = -2f * (1 - settle.value)
                    },
                    inDeck = true,
                )
            }
            BasicText(
                "This is how people nearby see you.",
                style = Kinjo.body.copy(fontSize = 13.sp, color = Kinjo.Muted),
                modifier = Modifier.align(Alignment.CenterHorizontally).padding(top = 6.dp, bottom = 14.dp),
            )
            KButton("Edit profile", onEdit, kind = ButtonKind.Line, modifier = Modifier.padding(bottom = 16.dp))
        }
    }
}
