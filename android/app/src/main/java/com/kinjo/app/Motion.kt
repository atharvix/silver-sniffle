package com.kinjo.app

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.BlurredEdgeTreatment
import androidx.compose.ui.draw.blur
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.drawscope.scale
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** The old app's motion curve: cubic-bezier(.22, 1, .36, 1), quick start, long soft landing. */
val Ease = CubicBezierEasing(0.22f, 1f, 0.36f, 1f)

private val MARK = Color(0xFFF2F0EB)

// The Kinjo mark: two halves in a 100 x 100 box, from the old splash SVG.
private val LEFT_HALF = PathParser().parsePathString("M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z").toPath()
private val RIGHT_HALF = PathParser().parsePathString("M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z").toPath()

/**
 * The old splash: the two halves of the mark slide in from opposite corners and join, the mark snaps,
 * then the whole screen lifts away. The app loads underneath meanwhile.
 */
@Composable
fun Splash(onDone: () -> Unit) {
    val left = remember { Animatable(0f) }
    val right = remember { Animatable(0f) }
    val snap = remember { Animatable(1f) }
    val lift = remember { Animatable(0f) }
    LaunchedEffect(Unit) {
        launch { delay(150); left.animateTo(1f, tween(1150, easing = Ease)) }
        launch { delay(250); right.animateTo(1f, tween(1150, easing = Ease)) }
        delay(1200)
        snap.animateTo(1.05f, tween(245, easing = Ease))
        snap.animateTo(1f, tween(455, easing = Ease))
        lift.animateTo(1f, tween(1000, easing = Ease))
        onDone()
    }
    Box(
        Modifier
            .fillMaxSize()
            .graphicsLayer { translationY = -size.height * 1.01f * lift.value }
            .background(Kinjo.Bg)
            .pointerInput(Unit) {}, // the screen underneath isn't tappable until the splash is gone
        contentAlignment = Alignment.Center,
    ) {
        Canvas(Modifier.size(124.dp).graphicsLayer { scaleX = snap.value; scaleY = snap.value }) {
            val u = size.width / 100f
            translate(-26 * u * (1 - left.value), 26 * u * (1 - left.value)) {
                scale(u, Offset.Zero) { drawPath(LEFT_HALF, MARK.copy(alpha = left.value)) }
            }
            translate(26 * u * (1 - right.value), -26 * u * (1 - right.value)) {
                scale(u, Offset.Zero) { drawPath(RIGHT_HALF, MARK.copy(alpha = right.value)) }
            }
        }
    }
}

/** A title that slides up out of a mask when its screen appears (the old `.rv`). */
@Composable
fun RevealText(text: String, style: TextStyle, modifier: Modifier = Modifier, delayMs: Int = 180) {
    val shown = remember { Animatable(0f) }
    LaunchedEffect(Unit) { shown.animateTo(1f, tween(1050, delayMillis = delayMs, easing = Ease)) }
    Box(modifier.clipToBounds()) {
        BasicText(text, style = style, modifier = Modifier.graphicsLayer { translationY = size.height * 1.06f * (1 - shown.value) })
    }
}

/** Content that fades in while rising 14 dp, or sliding in 18 dp from the right with `fromSide` (the old `.rf`). */
@Composable
fun Rise(modifier: Modifier = Modifier, delayMs: Int = 300, fromSide: Boolean = false, content: @Composable () -> Unit) {
    val shown = remember { Animatable(0f) }
    LaunchedEffect(Unit) { shown.animateTo(1f, tween(850, delayMillis = delayMs, easing = Ease)) }
    Box(
        modifier.graphicsLayer {
            alpha = shown.value
            val offset = (if (fromSide) 18.dp else 14.dp).toPx() * (1 - shown.value)
            if (fromSide) translationX = offset else translationY = offset
        },
    ) { content() }
}

/** Words that arrive one after another, each rising out of a slight blur (the old empty-state `.words`). */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun BlurInWords(text: String, style: TextStyle, modifier: Modifier = Modifier) {
    FlowRow(modifier) {
        val words = text.split(" ")
        words.forEachIndexed { i, word ->
            val shown = remember { Animatable(0f) }
            LaunchedEffect(Unit) { shown.animateTo(1f, tween(800, delayMillis = 300 + i * 70, easing = Ease)) }
            BasicText(
                if (i < words.lastIndex) "$word " else word, // the space travels with its word
                style = style,
                modifier = Modifier
                    .graphicsLayer {
                        alpha = shown.value
                        translationY = size.height * 0.42f * (1 - shown.value)
                    }
                    .blur((7 * (1 - shown.value)).dp, BlurredEdgeTreatment.Unbounded), // Android 12+; older phones just rise
            )
        }
    }
}
