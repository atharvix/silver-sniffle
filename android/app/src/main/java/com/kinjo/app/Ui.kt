package com.kinjo.app

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.BiasAlignment
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.em
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.TransformOrigin
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

@Composable
fun Eyebrow(text: String, color: Color = Kinjo.Muted) {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(Modifier.size(8.dp).background(color))
        BasicText(text.uppercase(), style = Kinjo.eyebrow.copy(color = color))
    }
}

enum class ButtonKind { Solid, Line, Danger }

@Composable
fun KButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, kind: ButtonKind = ButtonKind.Solid) {
    val look = when (kind) {
        ButtonKind.Solid -> Modifier.background(Kinjo.Fg)
        ButtonKind.Line -> Modifier.border(1.dp, Kinjo.Hairline)
        ButtonKind.Danger -> Modifier.background(Kinjo.Danger)
    }
    Box(
        modifier
            .fillMaxWidth()
            .height(56.dp)
            .alpha(if (enabled) 1f else 0.35f)
            .then(look)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) { BasicText(text, style = Kinjo.button.copy(color = if (kind == ButtonKind.Solid) Kinjo.Bg else Kinjo.Fg)) }
}

enum class Icon { Back, Close }

/** The old app's thin-line icon buttons (40 dp target, 20 dp drawing). */
@Composable
fun IconButton(icon: Icon, label: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Box(
        modifier
            .size(44.dp)
            .clickable(role = Role.Button, onClickLabel = label, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        Canvas(Modifier.size(20.dp)) {
            val u = size.width / 20f
            val stroke = Stroke(width = 1.8.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round)
            val path = Path().apply {
                when (icon) {
                    Icon.Back -> {
                        moveTo(16 * u, 10 * u); lineTo(4 * u, 10 * u)
                        moveTo(9 * u, 5 * u); lineTo(4 * u, 10 * u); lineTo(9 * u, 15 * u)
                    }
                    Icon.Close -> {
                        moveTo(5 * u, 5 * u); lineTo(15 * u, 15 * u)
                        moveTo(15 * u, 5 * u); lineTo(5 * u, 15 * u)
                    }
                }
            }
            drawPath(path, Kinjo.Fg, style = stroke)
        }
    }
}

/** Round photo, or the first letters of the name when there's no photo. */
@Composable
fun Avatar(photo: ImageBitmap?, name: String, size: Dp, modifier: Modifier = Modifier) {
    Box(modifier.size(size).clip(CircleShape).background(Kinjo.Surface2), contentAlignment = Alignment.Center) {
        if (photo != null) {
            Image(photo, contentDescription = null, contentScale = ContentScale.Crop, alignment = BiasAlignment(0f, -0.24f), modifier = Modifier.fillMaxSize())
        } else {
            val initials = name.split(Regex("\\s+")).filter { it.isNotEmpty() }.take(2).joinToString("") { it.take(1).uppercase() }
            BasicText(initials, style = Kinjo.body.copy(fontSize = (size.value * 0.33f).sp, fontWeight = FontWeight.Bold, letterSpacing = (-0.04).em))
        }
    }
}

/** Bottom sheet over a dim scrim; tapping the scrim dismisses it. */
@Composable
fun Sheet(onDismiss: () -> Unit, content: @Composable ColumnScope.() -> Unit) {
    Box(Modifier.fillMaxSize()) {
        Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.6f)).clickable(interactionSource = null, indication = null, onClick = onDismiss))
        Column(
            Modifier
                .align(Alignment.BottomCenter)
                .fillMaxWidth()
                .background(Kinjo.Bg)
                .drawBehind { drawLine(Kinjo.Hairline, Offset.Zero, Offset(size.width, 0f), 1.dp.toPx()) }
                .clickable(interactionSource = null, indication = null) {} // taps on the sheet stay on it
                .padding(start = 22.dp, end = 22.dp, top = 22.dp, bottom = 28.dp),
            content = content,
        )
    }
}

/** One message at a time, the old app's white bar near the bottom. */
val toast = MutableStateFlow<String?>(null)

fun showToast(message: String) {
    toast.value = message
}

@Composable
fun ToastHost(modifier: Modifier = Modifier) {
    val message by toast.collectAsState()
    LaunchedEffect(message) {
        if (message != null) {
            delay(3_500)
            toast.value = null
        }
    }
    message?.let {
        Row(
            modifier
                .padding(start = 18.dp, end = 18.dp, bottom = 16.dp)
                .fillMaxWidth()
                .background(Kinjo.Fg)
                .padding(horizontal = 16.dp, vertical = 15.dp)
                .semantics { liveRegion = LiveRegionMode.Polite },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Box(Modifier.size(7.dp).background(Kinjo.Bg))
            BasicText(it, style = Kinjo.body.copy(fontSize = 14.sp, color = Kinjo.Bg))
        }
    }
}

/** The usual message when the phone can't reach the server. */
const val OFFLINE = "Couldn’t reach Kinjo. Check your connection and try again."

const val TERMS_URL = "https://kinjo.world/terms-conditions"
const val PRIVACY_URL = "https://kinjo.world/privacy-policy"

fun openUrl(context: Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    } catch (e: ActivityNotFoundException) {
        showToast("No browser found to open the page.")
    }
}

/** The old app's header word mark. */
@Composable
fun Wordmark() {
    BasicText("KINJO", style = Kinjo.body.copy(fontSize = 22.sp, fontWeight = FontWeight.Bold, letterSpacing = (-0.04).em, lineHeight = 1.em))
}

/** Square tick box in the app's style; the row around it handles the tap. */
@Composable
fun KCheck(checked: Boolean, modifier: Modifier = Modifier) {
    Box(
        modifier.size(20.dp).border(1.dp, if (checked) Kinjo.Fg else Kinjo.Hairline).background(if (checked) Kinjo.Fg else Color.Transparent),
        contentAlignment = Alignment.Center,
    ) {
        if (checked) {
            Canvas(Modifier.size(12.dp)) {
                val path = Path().apply {
                    moveTo(size.width * 0.1f, size.height * 0.55f)
                    lineTo(size.width * 0.4f, size.height * 0.82f)
                    lineTo(size.width * 0.92f, size.height * 0.18f)
                }
                drawPath(path, Kinjo.Bg, style = Stroke(width = 2.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round))
            }
        }
    }
}

/** Text link styled like the old app's inline buttons ("Upload photo"). */
@Composable
fun KLink(text: String, onClick: () -> Unit) {
    BasicText(
        text,
        style = Kinjo.small.copy(color = Kinjo.Fg),
        modifier = Modifier.clickable(role = Role.Button, onClick = onClick).padding(vertical = 8.dp),
    )
}

fun wordCount(text: String) = text.split(Regex("\\s+")).count { it.isNotEmpty() }

/** Filled field with a floating label; `maxWords` adds the "n / max words" counter. */
@Composable
fun Field(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    hint: String? = null,
    maxWords: Int? = null,
    multiline: Boolean = false,
) {
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val raised by animateFloatAsState(if (focused || value.isNotEmpty()) 1f else 0f, label = "label")
    val over = maxWords != null && wordCount(value) > maxWords
    val edge = when {
        over -> Kinjo.Danger
        focused -> Kinjo.Fg
        else -> Color.Transparent
    }
    Column {
        // The label lives in the decoration so a tap anywhere on the box focuses the field.
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            singleLine = !multiline,
            textStyle = Kinjo.body,
            cursorBrush = SolidColor(Kinjo.Fg),
            interactionSource = interaction,
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = if (multiline) 118.dp else 62.dp)
                .background(Kinjo.Surface)
                .border(1.dp, edge),
            decorationBox = { input ->
                Box {
                    BasicText(
                        label,
                        style = Kinjo.body.copy(color = Kinjo.Muted),
                        modifier = Modifier
                            .padding(start = 16.dp, top = 19.dp)
                            .graphicsLayer {
                                transformOrigin = TransformOrigin(0f, 0.5f)
                                translationY = -12.dp.toPx() * raised
                                scaleX = 1f - 0.24f * raised
                                scaleY = scaleX
                            },
                    )
                    Box(Modifier.padding(start = 16.dp, end = 16.dp, top = 26.dp, bottom = 8.dp)) { input() }
                }
            },
        )
        if (hint != null || maxWords != null) {
            Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.SpaceBetween) {
                BasicText(hint.orEmpty(), style = Kinjo.small, modifier = Modifier.weight(1f))
                if (maxWords != null) {
                    BasicText(
                        "${wordCount(value)} / $maxWords words",
                        style = Kinjo.small.copy(color = if (over) Kinjo.Danger else Kinjo.Faint),
                    )
                }
            }
        }
    }
}

/**
 * A profile card as other people see it. `inDeck` is the larger size used on the nearby screen;
 * `onMore` adds the "···" button (block, report).
 */
@Composable
fun PersonCard(
    photo: Painter?,
    name: String,
    role: String,
    looking: String,
    modifier: Modifier = Modifier,
    inDeck: Boolean = false,
    onMore: (() -> Unit)? = null,
) {
    val s = if (inDeck) 1.2f else 1f
    Box(modifier.aspectRatio(5f / 7f).clip(RoundedCornerShape(if (inDeck) 18.dp else 13.dp)).background(Kinjo.CardBg)) {
        if (photo != null) {
            Image(photo, contentDescription = "Photo of $name", contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
        } else {
            Box(
                Modifier.fillMaxSize().background(Brush.radialGradient(listOf(Color(0xFF2E2E2D), Color(0xFF131313)))),
                contentAlignment = Alignment.Center,
            ) { BasicText("ADD PHOTO", style = Kinjo.eyebrow.copy(color = Color(0xB0FFFFFF), fontSize = 13.sp)) }
        }
        Box(
            Modifier.fillMaxSize().background(
                Brush.verticalGradient(
                    0.44f to Color.Transparent,
                    0.62f to Color.Black.copy(alpha = 0.38f),
                    0.88f to Color.Black.copy(alpha = 0.86f),
                    1f to Color.Black.copy(alpha = 0.92f),
                ),
            ),
        )
        Column(Modifier.align(Alignment.BottomStart).padding(start = 15.dp * s, end = 15.dp * s, bottom = 16.dp * s)) {
            BasicText(name.ifBlank { "Your name" }, style = Kinjo.body.copy(fontSize = 15.sp * s, fontWeight = FontWeight.Bold, color = Color(0xFFF1F1F1)))
            BasicText(role.ifBlank { "What you do" }, style = Kinjo.body.copy(fontSize = 10.sp * s, fontWeight = FontWeight.Medium, color = Color(0xFFC4C4C4)))
            Spacer(Modifier.height(8.dp * s))
            BasicText(looking.ifBlank { "What you’re looking for" }, style = Kinjo.body.copy(fontSize = 11.sp * s, color = Color(0xFFD9D9D9)))
        }
        if (onMore != null) {
            Box(
                Modifier.align(Alignment.TopEnd).padding(8.dp).size(36.dp).clip(CircleShape)
                    .background(Color.Black.copy(alpha = 0.32f))
                    .clickable(role = Role.Button, onClickLabel = "Block or report $name", onClick = onMore)
                    .semantics { contentDescription = "Block or report $name" },
                contentAlignment = Alignment.Center,
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                    repeat(3) { Box(Modifier.size(3.5.dp).clip(CircleShape).background(Color(0xFFF1F1F1))) }
                }
            }
        }
    }
}

/** Centred card over the dimmed screen, as in the old app. With `onDismiss` it gets a Cancel link. */
@Composable
fun PermissionCard(title: String, body: String, button: String, onClick: () -> Unit, onDismiss: (() -> Unit)? = null) {
    Box(
        Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.6f)).padding(22.dp),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            Modifier
                .widthIn(max = 380.dp)
                .clip(RoundedCornerShape(14.dp))
                .background(Kinjo.Surface2)
                .border(1.dp, Kinjo.Soft, RoundedCornerShape(14.dp))
                .padding(start = 28.dp, end = 28.dp, top = 30.dp, bottom = 28.dp),
        ) {
            BasicText(title, style = Kinjo.cardTitle)
            Spacer(Modifier.height(12.dp))
            BasicText(body, style = Kinjo.cardBody)
            Spacer(Modifier.height(40.dp))
            KButton(button, onClick)
            if (onDismiss != null) {
                Box(Modifier.fillMaxWidth().padding(top = 4.dp), contentAlignment = Alignment.Center) {
                    BasicText(
                        "Cancel",
                        style = Kinjo.body.copy(fontSize = 15.sp, color = Kinjo.Muted),
                        modifier = Modifier.clickable(role = Role.Button, onClick = onDismiss).padding(14.dp),
                    )
                }
            }
        }
    }
}
