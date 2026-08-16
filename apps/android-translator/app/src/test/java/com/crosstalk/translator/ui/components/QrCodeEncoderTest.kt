package com.crosstalk.translator.ui.components

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class QrCodeEncoderTest {
    @Test
    fun encodesListenerUrlAsSquareMatrixWithBothTones() {
        val matrix = QrCodeEncoder.encode(
            "https://crosstalk-sfu.fly.dev/broadcast/listen/session?t=token",
            size = 192,
        )

        assertEquals(192, matrix.width)
        assertEquals(192, matrix.height)
        var dark = 0
        var light = 0
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                if (matrix[x, y]) dark++ else light++
            }
        }
        assertTrue(dark > 0)
        assertTrue(light > 0)
    }
}
