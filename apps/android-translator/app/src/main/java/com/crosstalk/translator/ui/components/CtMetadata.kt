package com.crosstalk.translator.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import com.crosstalk.translator.ui.theme.CtTheme

/**
 * Ruled key/value metadata pair. Values use mono for IDs/diagnostics.
 */
@Composable
fun CtMetadata(
    label: String,
    value: String,
    modifier: Modifier = Modifier,
    monoValue: Boolean = true,
    testTag: String? = null,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing

    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = spacing.space1)
            .semantics(mergeDescendants = true) {
                contentDescription = "$label $value"
            }
            .then(if (testTag != null) Modifier.testTag(testTag) else Modifier),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.Top,
    ) {
        Text(
            text = label,
            style = type.metadata,
            color = colors.textTertiary,
            modifier = Modifier.weight(0.4f),
        )
        Text(
            text = value,
            style = if (monoValue) type.code else type.bodyCompact,
            color = colors.textSecondary,
            modifier = Modifier.weight(0.6f),
        )
    }
}

@Composable
fun CtMetadataBlock(
    title: String? = null,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing
    Column(modifier = modifier.fillMaxWidth()) {
        if (title != null) {
            Text(
                text = title.uppercase(),
                style = type.eyebrow,
                color = colors.textTertiary,
                modifier = Modifier.padding(bottom = spacing.space2),
            )
        }
        content()
    }
}
