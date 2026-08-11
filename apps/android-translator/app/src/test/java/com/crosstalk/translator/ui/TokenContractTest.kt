package com.crosstalk.translator.ui

import com.crosstalk.translator.ui.theme.CtColors
import com.crosstalk.translator.ui.theme.CtShape
import com.crosstalk.translator.ui.theme.CtSpace
import com.crosstalk.translator.ui.theme.Oklch
import com.crosstalk.translator.ui.theme.ctTypography
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TokenContractTest {
    @Test
    fun accentIsExactlyCyan() {
        assertEquals("#3DE0F0", CtColors.ACCENT_HEX)
        assertEquals(0xFF3DE0F0, CtColors.AccentArgb)
        // Compose Color stores sRGB components; verify via packed ARGB constructor path.
        val accent = CtColors.dark().accent
        assertEquals(0x3D, (accent.red * 255f).toInt())
        assertEquals(0xE0, (accent.green * 255f).toInt())
        assertEquals(0xF0, (accent.blue * 255f).toInt())
    }

    @Test
    fun darkCanvasOklchConvertsToReviewedArgb() {
        // oklch(0.178 0.008 250) → #0F1115
        assertEquals(0xFF0F1115, CtColors.DarkCanvasArgb)
        assertEquals(0xFF0F1115, Oklch.toArgb(0.178, 0.008, 250.0))
    }

    @Test
    fun lightSurfaceIsWhite() {
        assertEquals(0xFFFFFFFF, CtColors.LightSurfaceArgb)
    }

    @Test
    fun spacingTokensAreRigid() {
        assertEquals(4, CtSpace.space1.value.toInt())
        assertEquals(8, CtSpace.space2.value.toInt())
        assertEquals(12, CtSpace.space3.value.toInt())
        assertEquals(16, CtSpace.space4.value.toInt())
        assertEquals(24, CtSpace.space6.value.toInt())
        assertEquals(32, CtSpace.space8.value.toInt())
        assertEquals(48, CtSpace.space12.value.toInt())
        assertEquals(16, CtSpace.gutter.value.toInt())
        assertEquals(44, CtSpace.minTarget.value.toInt())
        assertEquals(48, CtSpace.minRow.value.toInt())
    }

    @Test
    fun shapeTokensAreRigid() {
        assertEquals(2, CtShape.radiusSmall.value.toInt())
        assertEquals(2, CtShape.radiusMedium.value.toInt())
        assertEquals(1, CtShape.rule.value.toInt())
        assertEquals(2, CtShape.focusWidth.value.toInt())
    }

    @Test
    fun typeScaleUsesExactSizes() {
        val compact = ctTypography(compactWidth = true)
        val wide = ctTypography(compactWidth = false)
        assertEquals(22f, compact.pageTitle.fontSize.value)
        assertEquals(30f, wide.pageTitle.fontSize.value)
        assertEquals(15f, compact.section.fontSize.value)
        assertEquals(14f, compact.lede.fontSize.value)
        assertEquals(14f, compact.body.fontSize.value)
        assertEquals(13f, compact.bodyCompact.fontSize.value)
        assertEquals(13f, compact.control.fontSize.value)
        assertEquals(12f, compact.label.fontSize.value)
        assertEquals(11f, compact.metadata.fontSize.value)
        assertEquals(10f, compact.eyebrow.fontSize.value)
        assertEquals(22f, compact.data.fontSize.value)
        assertEquals(30f, wide.data.fontSize.value)
        assertEquals(12f, compact.code.fontSize.value)
    }

    @Test
    fun darkPrimaryTextMeetsAaOnCanvas() {
        val ratio = Oklch.contrastRatio(
            CtColors.DarkTextPrimaryArgb,
            CtColors.DarkCanvasArgb,
        )
        assertTrue("dark primary on canvas contrast=$ratio", ratio >= 4.5)
    }

    @Test
    fun darkAccentInkMeetsAaOnAccent() {
        val ratio = Oklch.contrastRatio(
            CtColors.AccentInkArgb,
            CtColors.AccentArgb,
        )
        assertTrue("accent ink on cyan contrast=$ratio", ratio >= 4.5)
    }

    @Test
    fun lightPrimaryTextMeetsAaOnCanvas() {
        val ratio = Oklch.contrastRatio(
            CtColors.LightTextPrimaryArgb,
            CtColors.LightCanvasArgb,
        )
        assertTrue("light primary on canvas contrast=$ratio", ratio >= 4.5)
    }

    @Test
    fun darkSecondaryTextMeetsAaOnCanvas() {
        val ratio = Oklch.contrastRatio(
            CtColors.DarkTextSecondaryArgb,
            CtColors.DarkCanvasArgb,
        )
        assertTrue("dark secondary on canvas contrast=$ratio", ratio >= 4.5)
    }

    @Test
    fun minTargetIsAtLeast44dp() {
        assertTrue(CtSpace.minTarget.value >= 44f)
        assertTrue(CtSpace.minRow.value >= 48f)
    }
}
