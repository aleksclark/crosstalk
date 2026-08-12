package com.crosstalk.translator.audio

import android.content.Context
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.os.Build
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Communication audio routing. On API 31+ uses [AudioManager.setCommunicationDevice].
 * Cleared on stop.
 */
class AudioRouteController(
    context: Context,
) {
    private val appContext = context.applicationContext
    private val audioManager =
        appContext.getSystemService(Context.AUDIO_SERVICE) as AudioManager
    private val applied = AtomicBoolean(false)

    fun applyPreferredRoute() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            val devices = audioManager.availableCommunicationDevices
            val preferred =
                devices.firstOrNull { it.type == AudioDeviceInfo.TYPE_BLUETOOTH_SCO }
                    ?: devices.firstOrNull { it.type == AudioDeviceInfo.TYPE_WIRED_HEADSET }
                    ?: devices.firstOrNull { it.type == AudioDeviceInfo.TYPE_BUILTIN_SPEAKER }
                    ?: devices.firstOrNull()
            if (preferred != null) {
                audioManager.setCommunicationDevice(preferred)
                applied.set(true)
            }
        } else {
            @Suppress("DEPRECATION")
            audioManager.isSpeakerphoneOn = true
            applied.set(true)
        }
    }

    fun clear() {
        if (!applied.getAndSet(false)) return
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            try {
                audioManager.clearCommunicationDevice()
            } catch (_: RuntimeException) {
                // Device may already be cleared.
            }
        } else {
            @Suppress("DEPRECATION")
            audioManager.isSpeakerphoneOn = false
        }
    }

    fun isApplied(): Boolean = applied.get()
}
