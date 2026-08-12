package com.crosstalk.translator.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.crosstalk.translator.ui.theme.CtTheme

enum class CtStatusTone {
    Neutral,
    Ok,
    Warning,
    Danger,
    Info,
    Accent,
}

/**
 * Status chip: icon mark + text + semantic color. Never color-only.
 */
@Composable
fun CtStatus(
    text: String,
    tone: CtStatusTone = CtStatusTone.Neutral,
    modifier: Modifier = Modifier,
    compact: Boolean = true,
    testTag: String? = null,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val shapes = CtTheme.shapes
    val spacing = CtTheme.spacing

    val (fg, bg) = when (tone) {
        CtStatusTone.Neutral -> colors.textSecondary to colors.raised
        CtStatusTone.Ok -> colors.statusOk to colors.statusOkBackground
        CtStatusTone.Warning -> colors.statusWarning to colors.statusWarningBackground
        CtStatusTone.Danger -> colors.statusDanger to colors.statusDangerBackground
        CtStatusTone.Info -> colors.statusInfo to colors.statusInfoBackground
        CtStatusTone.Accent -> colors.accentInk to colors.accent
    }

    val shape = if (compact) {
        RoundedCornerShape(shapes.radiusPill)
    } else {
        RoundedCornerShape(shapes.radiusSmall)
    }

    Row(
        modifier = modifier
            .clip(shape)
            .background(bg)
            .padding(horizontal = spacing.space2, vertical = spacing.space1)
            .semantics { contentDescription = text }
            .then(if (testTag != null) Modifier.testTag(testTag) else Modifier),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(spacing.space1),
    ) {
        // Explicit mark so state is never color-only.
        Box(
            modifier = Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(fg),
        )
        Text(
            text = text,
            style = type.label,
            color = fg,
        )
    }
}
