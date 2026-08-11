package com.crosstalk.translator.audio

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.os.Build
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

/**
 * Voice-communication audio focus for bidirectional translator audio.
 *
 * - MODE_IN_COMMUNICATION while held
 * - USAGE_VOICE_COMMUNICATION / CONTENT_TYPE_SPEECH
 * - Permanent loss is terminal (callback)
 * - Transient loss mutes capture; regain restores unless manually muted
 */
class AudioFocusController(
    context: Context,
    private val listener: Listener,
) {
    interface Listener {
        fun onTransientFocusLoss()
        fun onFocusGain()
        fun onPermanentFocusLoss()
    }

    private val appContext = context.applicationContext
    private val audioManager =
        appContext.getSystemService(Context.AUDIO_SERVICE) as AudioManager

    private val held = AtomicBoolean(false)
    private val previousMode = AtomicReference(AudioManager.MODE_NORMAL)
    private var focusRequest: AudioFocusRequest? = null

    private val focusChangeListener =
        AudioManager.OnAudioFocusChangeListener { change ->
            when (change) {
                AudioManager.AUDIOFOCUS_LOSS -> {
                    held.set(false)
                    listener.onPermanentFocusLoss()
                }
                AudioManager.AUDIOFOCUS_LOSS_TRANSIENT,
                AudioManager.AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK,
                -> {
                    listener.onTransientFocusLoss()
                }
                AudioManager.AUDIOFOCUS_GAIN -> {
                    listener.onFocusGain()
                }
            }
        }

    fun isHeld(): Boolean = held.get()

    fun request(): Boolean {
        if (held.get()) return true
        previousMode.set(audioManager.mode)
        audioManager.mode = AudioManager.MODE_IN_COMMUNICATION

        val result =
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                val attrs =
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_VOICE_COMMUNICATION)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                        .build()
                val req =
                    AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
                        .setAudioAttributes(attrs)
                        .setOnAudioFocusChangeListener(focusChangeListener)
                        .setAcceptsDelayedFocusGain(false)
                        .setWillPauseWhenDucked(false)
                        .build()
                focusRequest = req
                audioManager.requestAudioFocus(req)
            } else {
                @Suppress("DEPRECATION")
                audioManager.requestAudioFocus(
                    focusChangeListener,
                    AudioManager.STREAM_VOICE_CALL,
                    AudioManager.AUDIOFOCUS_GAIN,
                )
            }

        val ok = result == AudioManager.AUDIOFOCUS_REQUEST_GRANTED
        held.set(ok)
        if (!ok) {
            restoreMode()
        }
        return ok
    }

    fun abandon() {
        if (!held.getAndSet(false) && focusRequest == null) {
            restoreMode()
            return
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            focusRequest?.let { audioManager.abandonAudioFocusRequest(it) }
            focusRequest = null
        } else {
            @Suppress("DEPRECATION")
            audioManager.abandonAudioFocus(focusChangeListener)
        }
        restoreMode()
    }

    private fun restoreMode() {
        try {
            audioManager.mode = previousMode.get()
        } catch (_: RuntimeException) {
            audioManager.mode = AudioManager.MODE_NORMAL
        }
    }
}
