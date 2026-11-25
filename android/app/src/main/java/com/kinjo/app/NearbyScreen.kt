package com.kinjo.app

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.text.BasicText
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.heightIn
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.foundation.layout.offset
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import kotlinx.coroutines.launch
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.rememberAsyncImagePainter
import kotlinx.coroutines.delay

private const val SCAN_MS = 1_700

/** "Looking around": a line that fills while the first position is found. Waits while location isn't allowed. */
@Composable
fun ScanScreen(ready: Boolean, onDone: () -> Unit) {
    val progress by animateFloatAsState(if (ready) 1f else 0f, tween(SCAN_MS), label = "scan")
    // shortcut: fixed duration; ends when the first nearby list arrives once the server exists.
    LaunchedEffect(ready) {
        if (ready) {
            delay(SCAN_MS + 300L)
            onDone()
        }
    }
    Column(Modifier.fillMaxSize().padding(horizontal = 22.dp), verticalArrangement = Arrangement.Center) {
        Eyebrow("Kinjo")
        BasicText("Looking around", style = Kinjo.title, modifier = Modifier.padding(top = 18.dp))
        BasicText("Finding your people within 30 metres.", style = Kinjo.sub, modifier = Modifier.padding(top = 14.dp))
        Box(Modifier.padding(top = 34.dp).fillMaxWidth().height(1.dp).background(Kinjo.Soft)) {
            Box(Modifier.fillMaxWidth(progress).fillMaxHeight().background(Kinjo.Fg))
        }
        BasicText("Finding your location", style = Kinjo.small, modifier = Modifier.padding(top = 12.dp))
    }
}

/** People within 30 m, live from the server; the empty or hidden state otherwise. */
@Composable
fun NearbyScreen(visible: Boolean, onLookAgain: () -> Unit, onMenu: () -> Unit, onShowProfile: () -> Unit) {
    val cards by nearbyCards.collectAsState()
    var acting by remember { mutableStateOf<Card?>(null) }
    Box(Modifier.fillMaxSize()) {
        Column(Modifier.fillMaxSize()) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 22.dp).height(56.dp), verticalAlignment = Alignment.CenterVertically) {
                Wordmark()
                Box(Modifier.weight(1f))
                MenuButton(onMenu)
            }
            Box(Modifier.weight(1f)) {
                when {
                    !visible -> Hidden(onShowProfile)
                    cards.isEmpty() -> Nobody(onLookAgain)
                    else -> Deck(cards, onMore = { acting = it })
                }
            }
        }
        acting?.let { CardActions(it, onDone = { acting = null }) }
    }
}

private val REPORT_REASONS = listOf(
    "fake" to "Fake profile",
    "inappropriate" to "Inappropriate photo or text",
    "harassment" to "Harassment",
    "spam" to "Spam or selling",
    "underage" to "Under 18",
    "other" to "Something else",
)

/** Block or report someone from their card. Both hide the two of you from each other at once. */
@Composable
private fun CardActions(card: Card, onDone: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var reporting by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }

    fun send(action: suspend () -> Unit, thanks: String) {
        if (busy) return
        busy = true
        scope.launch {
            val problem = tryServer(action)
            busy = false
            showToast(problem ?: thanks)
            if (problem == null) onDone()
        }
    }

    Sheet(onDismiss = { if (!busy) onDone() }) {
        Row(Modifier.fillMaxWidth().padding(bottom = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            Eyebrow(if (reporting) "Report ${card.name}" else card.name)
            Box(Modifier.weight(1f))
            IconButton(Icon.Close, "Close", { if (!busy) onDone() }, Modifier.offset(x = 12.dp))
        }
        if (!reporting) {
            BasicText("You won’t see each other again.", style = Kinjo.body.copy(fontSize = 14.5.sp, color = Kinjo.Muted))
            KButton("Block", { send({ Api.block(context, card.uid) }, "Blocked.") }, Modifier.padding(top = 22.dp), enabled = !busy)
            KButton("Report", { reporting = true }, Modifier.padding(top = 10.dp), enabled = !busy, kind = ButtonKind.Line)
        } else {
            BasicText("What’s wrong? They’re blocked too.", style = Kinjo.body.copy(fontSize = 14.5.sp, color = Kinjo.Muted))
            REPORT_REASONS.forEach { (reason, label) ->
                Row(
                    Modifier.fillMaxWidth().heightIn(min = 52.dp)
                        .clickable(enabled = !busy, role = Role.Button) { send({ Api.report(context, card.uid, reason) }, "Thanks. We’ll look into it.") },
                    verticalAlignment = Alignment.CenterVertically,
                ) { BasicText(label, style = Kinjo.body) }
                Box(Modifier.fillMaxWidth().height(1.dp).background(Kinjo.Soft))
            }
        }
    }
}

/** Three lines, the old app's menu mark. */
@Composable
private fun MenuButton(onClick: () -> Unit) {
    Column(
        Modifier
            .size(44.dp)
            .clickable(role = Role.Button, onClickLabel = "Open menu", onClick = onClick)
            .semantics { contentDescription = "Menu" }
            .padding(end = 4.dp),
        verticalArrangement = Arrangement.spacedBy(5.dp, Alignment.CenterVertically),
        horizontalAlignment = Alignment.End,
    ) {
        listOf(22.dp, 16.dp, 22.dp).forEach { Box(Modifier.size(width = it, height = 1.6.dp).background(Kinjo.Fg)) }
    }
}

@Composable
private fun Hidden(onShowProfile: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(horizontal = 22.dp).padding(top = 30.dp)) {
        Eyebrow("Profile hidden")
        BasicText("You’re hidden.", style = Kinjo.title, modifier = Modifier.padding(top = 18.dp))
        BasicText(
            "Nobody nearby can see you, and you won’t see them. Turn your profile on to find your people.",
            style = Kinjo.body.copy(fontSize = 17.sp, color = Kinjo.Muted),
            modifier = Modifier.padding(top = 16.dp).widthIn(max = 320.dp),
        )
        KButton("Show my profile", onShowProfile, Modifier.padding(top = 28.dp).widthIn(max = 200.dp).height(50.dp))
    }
}

@Composable
private fun Deck(cards: List<Card>, onMore: (Card) -> Unit) {
    val pager = rememberPagerState { cards.size }
    Column(Modifier.fillMaxSize().padding(start = 22.dp, end = 22.dp, bottom = 44.dp)) {
        Column(Modifier.padding(top = 8.dp)) {
            BasicText("Around you, right now", style = Kinjo.title.copy(fontSize = 30.sp, color = Kinjo.Muted))
            BasicText("Look up. People are right here.", style = Kinjo.title.copy(fontSize = 30.sp))
        }
        HorizontalPager(
            state = pager,
            key = { cards[it].uid },
            contentPadding = PaddingValues(horizontal = 30.dp),
            pageSpacing = 16.dp,
            modifier = Modifier.weight(1f).fillMaxWidth(),
        ) { page ->
            val card = cards[page]
            // Pressing a card lifts it by about 1 dp on each side.
            var pressed by remember { mutableStateOf(false) }
            val lift by animateFloatAsState(if (pressed) 1.008f else 1f, label = "lift")
            Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                PersonCard(
                    rememberAsyncImagePainter(card.img), card.name, card.role, card.looking,
                    Modifier.widthIn(max = 280.dp)
                        .graphicsLayer { scaleX = lift; scaleY = lift }
                        .pointerInput(Unit) {
                            detectTapGestures(onPress = {
                                pressed = true
                                tryAwaitRelease()
                                pressed = false
                            })
                        },
                    inDeck = true,
                    onMore = { onMore(card) },
                )
            }
        }
    }
}

@Composable
private fun Nobody(onLookAgain: () -> Unit) {
    val blink by rememberInfiniteTransition(label = "blink").animateFloat(
        initialValue = 1f,
        targetValue = 0.2f,
        animationSpec = infiniteRepeatable(tween(1_200), RepeatMode.Reverse),
        label = "blink",
    )
    Column(Modifier.fillMaxSize().padding(start = 22.dp, end = 22.dp, bottom = 56.dp), verticalArrangement = Arrangement.Center) {
        BasicText("Here is no one?", style = Kinjo.emptyTitle)
        BasicText(
            "Just move, and find your people.",
            style = Kinjo.words,
            modifier = Modifier.padding(top = 18.dp).widthIn(max = 280.dp),
        )
        Row(Modifier.padding(top = 34.dp), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Box(Modifier.size(6.dp).alpha(blink).background(Kinjo.Fg))
            BasicText("Still looking around you", style = Kinjo.small)
        }
        KButton("Look again", onLookAgain, Modifier.padding(top = 26.dp).widthIn(max = 160.dp).height(48.dp))
    }
}
