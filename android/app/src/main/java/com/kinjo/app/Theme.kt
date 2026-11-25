package com.kinjo.app

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp

/** Colours and type from the old Kinjo app (dark only). */
object Kinjo {
    val Bg = Color(0xFF0A0A0A)
    val Fg = Color.White
    val Surface = Color(0xFF141414)
    val Surface2 = Color(0xFF1C1C1C)
    val CardBg = Color(0xFF1B1B1B)
    val Hairline = Color(0x24FFFFFF)
    val Soft = Color(0x14FFFFFF)
    val Muted = Color(0x9EFFFFFF)
    val Faint = Color(0x61FFFFFF)
    val Danger = Color(0xFFFF5D5D)

    @OptIn(ExperimentalTextApi::class) // weight axis of the variable DM Sans file
    private val dmSans = FontFamily(
        listOf(400, 500, 700).map { w ->
            Font(R.font.dm_sans, FontWeight(w), variationSettings = FontVariation.Settings(FontVariation.weight(w)))
        },
    )

    private val base = TextStyle(fontFamily = dmSans, color = Fg, fontSize = 16.sp, lineHeight = 1.5.em)

    val body = base
    val title = base.copy(fontSize = 40.sp, letterSpacing = (-0.045).em, lineHeight = 1.04.em)
    val sub = base.copy(fontSize = 15.5.sp, color = Muted)
    val eyebrow = base.copy(fontSize = 11.sp, letterSpacing = 0.16.em, lineHeight = 1.em)
    val button = base.copy(color = Bg, letterSpacing = 0.005.em)
    val small = base.copy(fontSize = 12.5.sp, color = Faint)
    val cardTitle = base.copy(fontSize = 28.sp, letterSpacing = (-0.04).em, lineHeight = 1.15.em)
    val cardBody = base.copy(fontSize = 15.sp, color = Muted, lineHeight = 1.6.em)
    val emptyTitle = base.copy(fontSize = 52.sp, letterSpacing = (-0.05).em, lineHeight = 1.em)
    val words = base.copy(fontSize = 26.sp, fontWeight = FontWeight.Medium, letterSpacing = (-0.035).em, lineHeight = 1.2.em)
}
