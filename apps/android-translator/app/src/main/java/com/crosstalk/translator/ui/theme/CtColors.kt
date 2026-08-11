package com.crosstalk.translator.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Editorial Instrument color roles.
 *
 * Product accent is locked to cyan #3DE0F0 (the only brand accent).
 * Status hues are semantic only — never a second brand accent.
 *
 * ARGB constants are derived once from house OKLCH via [Oklch.toArgb];
 * source OKLCH is retained in comments for audit.
 */
@Immutable
data class CtColorScheme(
    val canvas: Color,
    val sunken: Color,
    val surface: Color,
    val raised: Color,
    val ruleSubtle: Color,
    val ruleStrong: Color,
    val textPrimary: Color,
    val textSecondary: Color,
    val textTertiary: Color,
    val accent: Color,
    val accentInk: Color,
    val focus: Color,
    val statusOk: Color,
    val statusOkBackground: Color,
    val statusWarning: Color,
    val statusWarningBackground: Color,
    val statusDanger: Color,
    val statusDangerBackground: Color,
    val statusInfo: Color,
    val statusInfoBackground: Color,
    val isDark: Boolean,
)

object CtColors {
    /** Selected product accent — cyan only. */
    const val ACCENT_HEX: String = "#3DE0F0"
    val AccentArgb: Long = 0xFF3DE0F0
    val AccentInkArgb: Long = 0xFF15170E

    // Dark roles — OKLCH from house-tokens.json
    // canvas oklch(0.178 0.008 250)
    val DarkCanvasArgb: Long = Oklch.toArgb(0.178, 0.008, 250.0)
    // sunken oklch(0.152 0.008 250)
    val DarkSunkenArgb: Long = Oklch.toArgb(0.152, 0.008, 250.0)
    // surface oklch(0.212 0.009 250)
    val DarkSurfaceArgb: Long = Oklch.toArgb(0.212, 0.009, 250.0)
    // raised oklch(0.252 0.010 250)
    val DarkRaisedArgb: Long = Oklch.toArgb(0.252, 0.010, 250.0)
    // ruleSubtle oklch(0.298 0.010 250)
    val DarkRuleSubtleArgb: Long = Oklch.toArgb(0.298, 0.010, 250.0)
    // ruleStrong oklch(0.392 0.012 250)
    val DarkRuleStrongArgb: Long = Oklch.toArgb(0.392, 0.012, 250.0)
    // textPrimary oklch(0.955 0.005 250)
    val DarkTextPrimaryArgb: Long = Oklch.toArgb(0.955, 0.005, 250.0)
    // textSecondary oklch(0.822 0.007 250)
    val DarkTextSecondaryArgb: Long = Oklch.toArgb(0.822, 0.007, 250.0)
    // textTertiary oklch(0.668 0.009 250)
    val DarkTextTertiaryArgb: Long = Oklch.toArgb(0.668, 0.009, 250.0)

    // Light roles
    val LightCanvasArgb: Long = Oklch.toArgb(0.985, 0.003, 250.0)
    val LightSunkenArgb: Long = Oklch.toArgb(0.952, 0.004, 250.0)
    val LightSurfaceArgb: Long = Oklch.toArgb(1.0, 0.0, 0.0)
    val LightRaisedArgb: Long = Oklch.toArgb(0.938, 0.005, 250.0)
    val LightRuleSubtleArgb: Long = Oklch.toArgb(0.882, 0.006, 250.0)
    val LightRuleStrongArgb: Long = Oklch.toArgb(0.782, 0.010, 250.0)
    val LightTextPrimaryArgb: Long = Oklch.toArgb(0.215, 0.012, 250.0)
    val LightTextSecondaryArgb: Long = Oklch.toArgb(0.405, 0.012, 250.0)
    val LightTextTertiaryArgb: Long = Oklch.toArgb(0.538, 0.012, 250.0)

    // Status dark
    val DarkOkArgb: Long = Oklch.toArgb(0.80, 0.14, 152.0)
    val DarkOkBgArgb: Long = Oklch.toArgb(0.2526, 0.0173, 175.8)
    val DarkWarningArgb: Long = Oklch.toArgb(0.83, 0.15, 78.0)
    val DarkWarningBgArgb: Long = Oklch.toArgb(0.2562, 0.0111, 83.1)
    val DarkDangerArgb: Long = Oklch.toArgb(0.72, 0.16, 26.0)
    val DarkDangerBgArgb: Long = Oklch.toArgb(0.2430, 0.0150, 6.9)
    val DarkInfoArgb: Long = Oklch.toArgb(0.80, 0.08, 235.0)
    val DarkInfoBgArgb: Long = Oklch.toArgb(0.2402, 0.0151, 242.1)

    // Status light
    val LightOkArgb: Long = Oklch.toArgb(0.48, 0.13, 152.0)
    val LightOkBgArgb: Long = Oklch.toArgb(0.9395, 0.0116, 165.4)
    val LightWarningArgb: Long = Oklch.toArgb(0.52, 0.13, 68.0)
    val LightWarningBgArgb: Long = Oklch.toArgb(0.9431, 0.0090, 67.4)
    val LightDangerArgb: Long = Oklch.toArgb(0.50, 0.18, 26.0)
    val LightDangerBgArgb: Long = Oklch.toArgb(0.9462, 0.0126, 17.2)
    val LightInfoArgb: Long = Oklch.toArgb(0.45, 0.09, 235.0)
    val LightInfoBgArgb: Long = Oklch.toArgb(0.9422, 0.0099, 239.1)

    fun dark(): CtColorScheme = CtColorScheme(
        canvas = Color(DarkCanvasArgb),
        sunken = Color(DarkSunkenArgb),
        surface = Color(DarkSurfaceArgb),
        raised = Color(DarkRaisedArgb),
        ruleSubtle = Color(DarkRuleSubtleArgb),
        ruleStrong = Color(DarkRuleStrongArgb),
        textPrimary = Color(DarkTextPrimaryArgb),
        textSecondary = Color(DarkTextSecondaryArgb),
        textTertiary = Color(DarkTextTertiaryArgb),
        accent = Color(AccentArgb),
        accentInk = Color(AccentInkArgb),
        focus = Color(AccentArgb),
        statusOk = Color(DarkOkArgb),
        statusOkBackground = Color(DarkOkBgArgb),
        statusWarning = Color(DarkWarningArgb),
        statusWarningBackground = Color(DarkWarningBgArgb),
        statusDanger = Color(DarkDangerArgb),
        statusDangerBackground = Color(DarkDangerBgArgb),
        statusInfo = Color(DarkInfoArgb),
        statusInfoBackground = Color(DarkInfoBgArgb),
        isDark = true,
    )

    fun light(): CtColorScheme = CtColorScheme(
        canvas = Color(LightCanvasArgb),
        sunken = Color(LightSunkenArgb),
        surface = Color(LightSurfaceArgb),
        raised = Color(LightRaisedArgb),
        ruleSubtle = Color(LightRuleSubtleArgb),
        ruleStrong = Color(LightRuleStrongArgb),
        textPrimary = Color(LightTextPrimaryArgb),
        textSecondary = Color(LightTextSecondaryArgb),
        textTertiary = Color(LightTextTertiaryArgb),
        accent = Color(AccentArgb),
        accentInk = Color(AccentInkArgb),
        focus = Color(AccentArgb),
        statusOk = Color(LightOkArgb),
        statusOkBackground = Color(LightOkBgArgb),
        statusWarning = Color(LightWarningArgb),
        statusWarningBackground = Color(LightWarningBgArgb),
        statusDanger = Color(LightDangerArgb),
        statusDangerBackground = Color(LightDangerBgArgb),
        statusInfo = Color(LightInfoArgb),
        statusInfoBackground = Color(LightInfoBgArgb),
        isDark = false,
    )
}

val LocalCtColors = staticCompositionLocalOf { CtColors.dark() }
