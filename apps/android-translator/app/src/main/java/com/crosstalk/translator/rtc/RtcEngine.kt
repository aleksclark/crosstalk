package com.crosstalk.translator.rtc

import kotlinx.coroutines.flow.Flow

/**
 * Production-neutral RTC boundary owned by TranslatorAudioService.
 *
 * Implementations:
 * - [LibWebRtcEngine] — final production wiring (main source set)
 * - FakeRtcEngine — test source set only; never linked into release APK
 */
interface RtcEngine {
    val events: Flow<RtcEvent>

    suspend fun connect(request: RtcConnectRequest)

    /** Disables/enables the local microphone track only; receive + signaling stay up. */
    suspend fun setMuted(muted: Boolean)

    suspend fun stats(): RtcStats

    /** Idempotent ordered teardown. */
    suspend fun close(reason: StopReason)
}
