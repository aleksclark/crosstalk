package com.crosstalk.translator.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.Modifier
import androidx.compose.foundation.background

object CtTheme {
    val colors: CtColorScheme
        @Composable
        @ReadOnlyComposable
        get() = LocalCtColors.current

    val typography: CtTypography
        @Composable
        @ReadOnlyComposable
        get() = LocalCtTypography.current

    val spacing: CtSpacing
        @Composable
        @ReadOnlyComposable
        get() = LocalCtSpacing.current

    val shapes: CtShapes
        @Composable
        @ReadOnlyComposable
        get() = LocalCtShapes.current
}

@Composable
fun CrossTalkTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val colors = if (darkTheme) CtColors.dark() else CtColors.light()
    val typography = rememberCtTypography()

    // Material3 bridge — map house roles so leftover Material widgets stay on-token.
    val material = if (darkTheme) {
        darkColorScheme(
            primary = colors.accent,
            onPrimary = colors.accentInk,
            secondary = colors.textSecondary,
            onSecondary = colors.canvas,
            background = colors.canvas,
            onBackground = colors.textPrimary,
            surface = colors.surface,
            onSurface = colors.textPrimary,
            surfaceVariant = colors.raised,
            onSurfaceVariant = colors.textSecondary,
            outline = colors.ruleStrong,
            error = colors.statusDanger,
            onError = colors.textPrimary,
        )
    } else {
        lightColorScheme(
            primary = colors.accent,
            onPrimary = colors.accentInk,
            secondary = colors.textSecondary,
            onSecondary = colors.canvas,
            background = colors.canvas,
            onBackground = colors.textPrimary,
            surface = colors.surface,
            onSurface = colors.textPrimary,
            surfaceVariant = colors.raised,
            onSurfaceVariant = colors.textSecondary,
            outline = colors.ruleStrong,
            error = colors.statusDanger,
            onError = colors.canvas,
        )
    }

    CompositionLocalProvider(
        LocalCtColors provides colors,
        LocalCtTypography provides typography,
        LocalCtSpacing provides CtSpace,
        LocalCtShapes provides CtShape,
    ) {
        MaterialTheme(colorScheme = material) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(colors.canvas),
            ) {
                content()
            }
        }
    }
}
