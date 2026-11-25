package com.kinjo.app

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.painter.BitmapPainter
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp

/** The old app's side menu: your avatar, name and role, then Settings. */
@Composable
fun Drawer(onClose: () -> Unit, onViewCard: () -> Unit, onSettings: () -> Unit) {
    val context = LocalContext.current
    val me = remember { ProfileStore.load(context) }
    val photo = remember { ProfileStore.photo(context) }
    Box(Modifier.fillMaxSize()) {
        Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.6f)).clickable(interactionSource = null, indication = null, onClick = onClose))
        Column(
            Modifier
                .align(Alignment.CenterEnd)
                .fillMaxHeight()
                .fillMaxWidth(0.82f)
                .background(Kinjo.Bg)
                .drawBehind { drawLine(Kinjo.Hairline, Offset.Zero, Offset(0f, size.height), 1.dp.toPx()) }
                .clickable(interactionSource = null, indication = null) {} // taps inside stay inside
                .padding(start = 24.dp, end = 24.dp, top = 8.dp),
        ) {
            IconButton(Icon.Close, "Close menu", onClose, Modifier.align(Alignment.End).offset(x = 12.dp))
            Column(Modifier.padding(top = 18.dp, bottom = 26.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Avatar(photo, me?.name.orEmpty(), 84.dp, Modifier.clickable(role = Role.Button, onClickLabel = "View my card", onClick = onViewCard))
                Column {
                    BasicText(me?.name.orEmpty(), style = Kinjo.body.copy(fontSize = 26.sp, letterSpacing = (-0.04).em, lineHeight = 1.1.em))
                    BasicText(me?.role.orEmpty(), style = Kinjo.body.copy(fontSize = 14.sp, color = Kinjo.Muted), modifier = Modifier.padding(top = 4.dp))
                }
            }
            Box(Modifier.fillMaxWidth().height(1.dp).background(Kinjo.Soft))
            Row(
                Modifier.fillMaxWidth().heightIn(min = 58.dp).clickable(role = Role.Button, onClick = onSettings),
                verticalAlignment = Alignment.CenterVertically,
            ) { BasicText("Settings", style = Kinjo.body) }
            Box(Modifier.fillMaxWidth().height(1.dp).background(Kinjo.Soft))
        }
    }
}

/** "Your card": exactly what people nearby see. */
@Composable
fun CardViewer(onClose: () -> Unit, onEdit: () -> Unit) {
    val context = LocalContext.current
    val me = remember { ProfileStore.load(context) }
    val photo = remember { ProfileStore.photo(context)?.let(::BitmapPainter) }
    Column(Modifier.fillMaxSize().background(Kinjo.Bg).clickable(interactionSource = null, indication = null) {}.padding(horizontal = 22.dp)) {
        Row(Modifier.fillMaxWidth().heightIn(min = 54.dp), verticalAlignment = Alignment.CenterVertically) {
            Eyebrow("Your card")
            Box(Modifier.weight(1f))
            IconButton(Icon.Close, "Close", onClose, Modifier.offset(x = 12.dp))
        }
        Box(Modifier.weight(1f).fillMaxWidth().padding(vertical = 12.dp), contentAlignment = Alignment.Center) {
            PersonCard(photo, me?.name.orEmpty(), me?.role.orEmpty(), me?.looking.orEmpty(), Modifier.widthIn(max = 314.dp), inDeck = true)
        }
        BasicText(
            "This is how people nearby see you.",
            style = Kinjo.body.copy(fontSize = 13.sp, color = Kinjo.Muted),
            modifier = Modifier.align(Alignment.CenterHorizontally).padding(top = 6.dp, bottom = 14.dp),
        )
        KButton("Edit profile", onEdit, kind = ButtonKind.Line, modifier = Modifier.padding(bottom = 16.dp))
    }
}
