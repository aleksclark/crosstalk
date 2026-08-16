package com.crosstalk.translator.ui.components

import android.graphics.Bitmap
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.foundation.Image
import com.crosstalk.translator.ui.theme.CtTheme
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.qrcode.QRCodeWriter
import com.google.zxing.qrcode.decoder.ErrorCorrectionLevel
import com.google.zxing.common.BitMatrix

object QrCodeEncoder {
    fun encode(value: String, size: Int): BitMatrix {
        require(value.isNotBlank()) { "QR value must not be blank" }
        require(size > 0) { "QR size must be positive" }
        return QRCodeWriter().encode(
            value,
            BarcodeFormat.QR_CODE,
            size,
            size,
            mapOf(
                EncodeHintType.ERROR_CORRECTION to ErrorCorrectionLevel.M,
                EncodeHintType.MARGIN to 2,
                EncodeHintType.CHARACTER_SET to Charsets.UTF_8.name(),
            ),
        )
    }
}

@Composable
fun CtQrCode(
    value: String,
    contentDescription: String,
    modifier: Modifier = Modifier,
    size: Dp = 192.dp,
    testTag: String = "qr_code",
) {
    val matrix = remember(value) { QrCodeEncoder.encode(value, 384) }
    val image = remember(matrix) {
        val pixels = IntArray(matrix.width * matrix.height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                pixels[y * matrix.width + x] = if (matrix[x, y]) {
                    android.graphics.Color.BLACK
                } else {
                    android.graphics.Color.WHITE
                }
            }
        }
        Bitmap.createBitmap(
            pixels,
            matrix.width,
            matrix.height,
            Bitmap.Config.ARGB_8888,
        ).asImageBitmap()
    }
    val spacing = CtTheme.spacing
    val shapes = CtTheme.shapes

    Box(
        modifier = modifier
            .background(Color.White, RoundedCornerShape(shapes.radiusSmall))
            .padding(spacing.space3),
    ) {
        Image(
            bitmap = image,
            contentDescription = contentDescription,
            modifier = Modifier
                .size(size)
                .testTag(testTag),
        )
    }
}
