package com.crosstalk.translator.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.crosstalk.translator.ui.theme.CtTheme
import kotlin.math.roundToInt

/**
 * Labelled level meter with numerical/semantic fallback.
 * Does not rely on animation or color alone for meaning.
 */
@Composable
fun CtMeter(
    label: String,
    level: Float,
    activeDescription: String,
    inactiveDescription: String,
    modifier: Modifier = Modifier,
    testTag: String? = null,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val shapes = CtTheme.shapes
    val spacing = CtTheme.spacing
    val clamped = level.coerceIn(0f, 1f)
    val percent = (clamped * 100f).roundToInt()
    val active = clamped >= 0.02f
    val semantic = if (active) activeDescription else inactiveDescription
    // Throttle semantics text to reduce TalkBack spam (bucketed percent).
    val bucket = remember(percent / 10) { (percent / 10) * 10 }
    val a11y = "$label: $semantic, $bucket percent"

    Column(
        modifier = modifier
            .semantics(mergeDescendants = true) {
                contentDescription = a11y
            }
            .then(if (testTag != null) Modifier.testTag(testTag) else Modifier),
    ) {
        Text(
            text = label.uppercase(),
            style = type.eyebrow,
            color = colors.textTertiary,
        )
        Spacer(modifier = Modifier.height(spacing.space1))
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(8.dp)
                .clip(RoundedCornerShape(shapes.radiusSmall))
                .background(colors.sunken),
        ) {
            Box(
                modifier = Modifier
                    .fillMaxHeight()
                    .fillMaxWidth(clamped.coerceAtLeast(0.01f).takeIf { active } ?: 0f)
                    .background(if (active) colors.accent else colors.ruleStrong),
            )
        }
        Spacer(modifier = Modifier.height(spacing.space1))
        Text(
            text = semantic,
            style = type.metadata,
            color = colors.textSecondary,
        )
    }
}
