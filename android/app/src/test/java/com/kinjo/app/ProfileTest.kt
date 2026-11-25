package com.kinjo.app

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ProfileTest {
    @Test
    fun countsWordsAcrossAnyWhitespace() {
        assertEquals(0, wordCount("   "))
        assertEquals(3, wordCount("  building \n in\thealthcare "))
    }

    @Test
    fun cardNeedsPhotoNameAndBothTextsWithinLimits() {
        val ok = Profile("Asha", "Building in healthcare", "A technical co-founder")
        assertTrue(ok.isComplete(hasPhoto = true))
        assertFalse(ok.isComplete(hasPhoto = false))
        assertFalse(ok.copy(name = " ").isComplete(hasPhoto = true))
        assertFalse(ok.copy(role = "").isComplete(hasPhoto = true))
        assertFalse(ok.copy(role = "one two three four five six seven eight").isComplete(hasPhoto = true))
        assertFalse(ok.copy(looking = (1..21).joinToString(" ") { "w" }).isComplete(hasPhoto = true))
    }
}
