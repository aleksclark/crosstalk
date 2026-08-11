package com.crosstalk.translator.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.crosstalk.translator.R

val ArchivoFamily = FontFamily(
    Font(R.font.archivo_regular, FontWeight.Normal),
    Font(R.font.archivo_medium, FontWeight.Medium),
    Font(R.font.archivo_semibold, FontWeight.SemiBold),
    Font(R.font.archivo_bold, FontWeight.Bold),
)

val NewsreaderFamily = FontFamily(
    Font(R.font.newsreader_regular, FontWeight.Normal),
)

val IbmPlexMonoFamily = FontFamily(
    Font(R.font.ibm_plex_mono_regular, FontWeight.Normal),
    Font(R.font.ibm_plex_mono_medium, FontWeight.Medium),
)

/**
 * Editorial Instrument type scale.
 * Mobile (compact width < 600dp) uses pageTitle/data 22sp; wider uses 30sp.
 */
@Immutable
data class CtTypography(
    val pageTitle: TextStyle,
    val section: TextStyle,
    val lede: TextStyle,
    val body: TextStyle,
    val bodyCompact: TextStyle,
    val control: TextStyle,
    val label: TextStyle,
    val metadata: TextStyle,
    val eyebrow: TextStyle,
    val data: TextStyle,
    val code: TextStyle,
    /** Login welcome only — Newsreader editorial face. */
    val editorialWelcome: TextStyle,
)

fun ctTypography(compactWidth: Boolean): CtTypography {
    val titleSize = if (compactWidth) 22.sp else 30.sp
    val dataSize = if (compactWidth) 22.sp else 30.sp
    return CtTypography(
        pageTitle = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Bold,
            fontSize = titleSize,
            lineHeight = titleSize * 1.12f,
            letterSpacing = (-0.022).emApprox(titleSize),
        ),
        section = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.SemiBold,
            fontSize = 15.sp,
            lineHeight = 15.sp * 1.2f,
        ),
        lede = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 14.sp,
            lineHeight = 14.sp * 1.55f,
        ),
        body = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 14.sp,
            lineHeight = 14.sp * 1.6f,
        ),
        bodyCompact = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 13.sp,
            lineHeight = 13.sp * 1.5f,
        ),
        control = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Medium,
            fontSize = 13.sp,
            lineHeight = 13.sp,
        ),
        label = TextStyle(
            fontFamily = ArchivoFamily,
            fontWeight = FontWeight.Medium,
            fontSize = 12.sp,
            lineHeight = 12.sp * 1.3f,
        ),
        metadata = TextStyle(
            fontFamily = IbmPlexMonoFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 11.sp,
            lineHeight = 11.sp * 1.5f,
        ),
        eyebrow = TextStyle(
            fontFamily = IbmPlexMonoFamily,
            fontWeight = FontWeight.Medium,
            fontSize = 10.sp,
            lineHeight = 10.sp,
            letterSpacing = 0.16.emApprox(10.sp),
        ),
        data = TextStyle(
            fontFamily = IbmPlexMonoFamily,
            fontWeight = FontWeight.Medium,
            fontSize = dataSize,
            lineHeight = dataSize,
            fontFeatureSettings = "tnum",
        ),
        code = TextStyle(
            fontFamily = IbmPlexMonoFamily,
            fontWeight = FontWeight.Normal,
            fontSize = 12.sp,
            lineHeight = 12.sp * 1.7f,
        ),
        editorialWelcome = TextStyle(
            fontFamily = NewsreaderFamily,
            fontWeight = FontWeight.Normal,
            fontSize = titleSize,
            lineHeight = titleSize * 1.12f,
        ),
    )
}

private fun Double.emApprox(size: androidx.compose.ui.unit.TextUnit): androidx.compose.ui.unit.TextUnit {
    // letterSpacing in sp ≈ em * fontSize
    return (this * size.value).sp
}

val LocalCtTypography = staticCompositionLocalOf { ctTypography(compactWidth = true) }

@Composable
fun rememberCtTypography(): CtTypography {
    val widthDp = LocalConfiguration.current.screenWidthDp
    return ctTypography(compactWidth = widthDp < 600)
}
