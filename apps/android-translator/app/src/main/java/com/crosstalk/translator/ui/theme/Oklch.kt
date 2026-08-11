package com.crosstalk.translator.ui.theme

import kotlin.math.cos
import kotlin.math.pow
import kotlin.math.roundToInt
import kotlin.math.sin

/**
 * Authoritative OKLCH → linear sRGB → 8-bit ARGB conversion used by [CtColors].
 * Source OKLCH values come from Editorial Instrument house-tokens.json.
 */
object Oklch {
    fun toArgb(L: Double, C: Double, H: Double, alpha: Int = 0xFF): Long {
        val hRad = Math.toRadians(H)
        val a = C * cos(hRad)
        val b = C * sin(hRad)

        val l_ = L + 0.3963377774 * a + 0.2158037573 * b
        val m_ = L - 0.1055613458 * a - 0.0638541728 * b
        val s_ = L - 0.0894841775 * a - 1.2914855480 * b

        val l = l_.pow(3.0)
        val m = m_.pow(3.0)
        val s = s_.pow(3.0)

        val rLin = +4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s
        val gLin = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s
        val bLin = -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s

        val r = (linearToSrgb(rLin) * 255.0).roundToInt().coerceIn(0, 255)
        val g = (linearToSrgb(gLin) * 255.0).roundToInt().coerceIn(0, 255)
        val bl = (linearToSrgb(bLin) * 255.0).roundToInt().coerceIn(0, 255)
        return ((alpha and 0xFF).toLong() shl 24) or
            (r.toLong() shl 16) or
            (g.toLong() shl 8) or
            bl.toLong()
    }

    private fun linearToSrgb(c: Double): Double {
        val clipped = c.coerceIn(0.0, 1.0)
        return if (clipped <= 0.0031308) {
            12.92 * clipped
        } else {
            1.055 * clipped.pow(1.0 / 2.4) - 0.055
        }
    }

    /** Relative luminance of an opaque ARGB color (sRGB). */
    fun relativeLuminance(argb: Long): Double {
        fun channel(v: Int): Double {
            val s = v / 255.0
            return if (s <= 0.04045) s / 12.92 else ((s + 0.055) / 1.055).pow(2.4)
        }
        val r = channel(((argb shr 16) and 0xFF).toInt())
        val g = channel(((argb shr 8) and 0xFF).toInt())
        val b = channel((argb and 0xFF).toInt())
        return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }

    fun contrastRatio(fgArgb: Long, bgArgb: Long): Double {
        val l1 = relativeLuminance(fgArgb)
        val l2 = relativeLuminance(bgArgb)
        val lighter = maxOf(l1, l2)
        val darker = minOf(l1, l2)
        return (lighter + 0.05) / (darker + 0.05)
    }
}
