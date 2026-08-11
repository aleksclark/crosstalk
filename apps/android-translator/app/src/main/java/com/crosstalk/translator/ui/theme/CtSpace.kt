package com.crosstalk.translator.ui.theme

import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * Rigid Editorial Instrument spacing + mobile touch floors.
 * Values are fixed tokens — do not invent intermediate spacing.
 */
@Immutable
data class CtSpacing(
    val space1: Dp = 4.dp,
    val space2: Dp = 8.dp,
    val space3: Dp = 12.dp,
    val space4: Dp = 16.dp,
    val space6: Dp = 24.dp,
    val space8: Dp = 32.dp,
    val space12: Dp = 48.dp,
    /** Mobile gutter floor. */
    val gutter: Dp = 16.dp,
    /** Minimum interactive control target. */
    val minTarget: Dp = 44.dp,
    /** Minimum list row height. */
    val minRow: Dp = 48.dp,
)

@Immutable
data class CtShapes(
    val radiusSmall: Dp = 2.dp,
    val radiusMedium: Dp = 2.dp,
    val radiusPill: Dp = 999.dp,
    val rule: Dp = 1.dp,
    val selectedRule: Dp = 2.dp,
    val focusWidth: Dp = 2.dp,
    val focusOffset: Dp = 2.dp,
)

val CtSpace = CtSpacing()
val CtShape = CtShapes()

val LocalCtSpacing = staticCompositionLocalOf { CtSpace }
val LocalCtShapes = staticCompositionLocalOf { CtShape }
