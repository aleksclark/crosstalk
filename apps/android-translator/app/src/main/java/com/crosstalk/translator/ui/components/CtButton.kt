package com.crosstalk.translator.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import com.crosstalk.translator.ui.theme.CtTheme

enum class CtButtonVariant {
    Primary,
    Secondary,
    Destructive,
    Ghost,
}

@Composable
fun CtButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    variant: CtButtonVariant = CtButtonVariant.Primary,
    enabled: Boolean = true,
    loading: Boolean = false,
    fillMaxWidth: Boolean = false,
    testTag: String? = null,
) {
    val colors = CtTheme.colors
    val shapes = CtTheme.shapes
    val spacing = CtTheme.spacing
    val type = CtTheme.typography
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val shape = RoundedCornerShape(shapes.radiusMedium)

    val bg: Color
    val fg: Color
    val border: BorderStroke?
    when (variant) {
        CtButtonVariant.Primary -> {
            bg = if (enabled) colors.accent else colors.raised
            fg = if (enabled) colors.accentInk else colors.textTertiary
            border = null
        }
        CtButtonVariant.Secondary -> {
            bg = Color.Transparent
            fg = if (enabled) colors.textPrimary else colors.textTertiary
            border = BorderStroke(shapes.rule, colors.ruleStrong)
        }
        CtButtonVariant.Destructive -> {
            bg = Color.Transparent
            fg = if (enabled) colors.statusDanger else colors.textTertiary
            border = BorderStroke(shapes.rule, colors.statusDanger)
        }
        CtButtonVariant.Ghost -> {
            bg = Color.Transparent
            fg = if (enabled) colors.textSecondary else colors.textTertiary
            border = null
        }
    }

    val focusBorder = if (focused) {
        BorderStroke(shapes.focusWidth, colors.focus)
    } else {
        border
    }

    val baseModifier = modifier
        .then(if (fillMaxWidth) Modifier.fillMaxWidth() else Modifier)
        .defaultMinSize(minWidth = spacing.minTarget, minHeight = spacing.minTarget)
        .clip(shape)
        .background(bg, shape)
        .then(
            if (focusBorder != null) {
                Modifier.border(focusBorder, shape)
            } else {
                Modifier
            },
        )
        .clickable(
            enabled = enabled && !loading,
            role = Role.Button,
            interactionSource = interaction,
            indication = null,
            onClick = onClick,
        )
        .focusable(enabled = enabled, interactionSource = interaction)
        .padding(horizontal = spacing.space4, vertical = spacing.space3)
        .semantics {
            role = Role.Button
            if (!enabled || loading) disabled()
        }
        .then(if (testTag != null) Modifier.testTag(testTag) else Modifier)

    Box(
        modifier = baseModifier,
        contentAlignment = Alignment.Center,
    ) {
        if (loading) {
            CircularProgressIndicator(
                modifier = Modifier.defaultMinSize(minWidth = 18.dp, minHeight = 18.dp),
                color = fg,
                strokeWidth = 2.dp,
            )
        } else {
            Text(
                text = text,
                style = type.control,
                color = fg,
                textAlign = TextAlign.Center,
            )
        }
    }
}
