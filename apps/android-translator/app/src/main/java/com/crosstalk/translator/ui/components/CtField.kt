package com.crosstalk.translator.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsFocusedAsState
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.error
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.VisualTransformation
import com.crosstalk.translator.ui.theme.CtTheme

/**
 * External-label field with sunken ground and inline error.
 */
@Composable
fun CtField(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    help: String? = null,
    error: String? = null,
    enabled: Boolean = true,
    singleLine: Boolean = true,
    visualTransformation: VisualTransformation = VisualTransformation.None,
    keyboardOptions: KeyboardOptions = KeyboardOptions.Default,
    keyboardActions: KeyboardActions = KeyboardActions.Default,
    testTag: String? = null,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val shapes = CtTheme.shapes
    val spacing = CtTheme.spacing
    val interaction = remember { MutableInteractionSource() }
    val focused by interaction.collectIsFocusedAsState()
    val shape = RoundedCornerShape(shapes.radiusSmall)
    val borderColor = when {
        error != null -> colors.statusDanger
        focused -> colors.focus
        else -> colors.ruleSubtle
    }
    val borderWidth = if (focused || error != null) shapes.focusWidth else shapes.rule

    Column(modifier = modifier.fillMaxWidth()) {
        Text(
            text = label,
            style = type.label,
            color = colors.textSecondary,
        )
        if (help != null) {
            Spacer(modifier = Modifier.height(spacing.space1))
            Text(
                text = help,
                style = type.metadata,
                color = colors.textTertiary,
            )
        }
        Spacer(modifier = Modifier.height(spacing.space1))
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            enabled = enabled,
            singleLine = singleLine,
            textStyle = type.body.copy(color = colors.textPrimary),
            cursorBrush = SolidColor(colors.accent),
            visualTransformation = visualTransformation,
            keyboardOptions = keyboardOptions,
            keyboardActions = keyboardActions,
            interactionSource = interaction,
            modifier = Modifier
                .fillMaxWidth()
                .background(colors.sunken, shape)
                .border(borderWidth, borderColor, shape)
                .padding(horizontal = spacing.space3, vertical = spacing.space3)
                .semantics {
                    contentDescription = label
                    if (error != null) error(error)
                }
                .then(if (testTag != null) Modifier.testTag(testTag) else Modifier),
        )
        if (error != null) {
            Spacer(modifier = Modifier.height(spacing.space1))
            Text(
                text = error,
                style = type.bodyCompact,
                color = colors.statusDanger,
                modifier = Modifier
                    .testTag((testTag ?: "field") + "_error")
                    .semantics {
                        error(error)
                        contentDescription = error
                    },
            )
        }
    }
}
