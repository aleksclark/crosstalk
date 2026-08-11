package com.crosstalk.translator.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.crosstalk.translator.ui.theme.CtTheme

@Composable
fun CtRule(
    modifier: Modifier = Modifier,
    strong: Boolean = false,
) {
    val colors = CtTheme.colors
    val shapes = CtTheme.shapes
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(shapes.rule)
            .background(if (strong) colors.ruleStrong else colors.ruleSubtle),
    )
}
