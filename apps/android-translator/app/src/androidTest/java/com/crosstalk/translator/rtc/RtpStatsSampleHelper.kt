package com.crosstalk.translator.rtc

import android.util.Log
import kotlinx.coroutines.delay

/**
 * Shared sampling helpers for production [LibWebRtcEngine.stats] assertions.
 *
 * Boundary note:
 * - Emulator capture is **synthetic-capture-debug-only** (silent/fake mic path).
 * - Outbound RTP counters can still advance when the peer connection is up even
 *   if the physical mic is silent (comfort noise / empty frames depending on ADM).
 * - Inbound bytes / [RtcStats.totalAudioEnergy] require a floor/feed peer on the
 *   server; when absent, callers must fail-soft with an explicit skip reason.
 */
object RtpStatsSampleHelper {
    const val LOG_TAG = "CT_GOLDEN_STATS"

    data class SampleWindow(
        val first: RtcStats,
        val last: RtcStats,
        val samples: List<RtcStats>,
    ) {
        val outboundAdvanced: Boolean
            get() =
                last.bytesSent > first.bytesSent ||
                    last.packetsSent > first.packetsSent

        val inboundAdvanced: Boolean
            get() =
                last.bytesReceived > first.bytesReceived ||
                    last.packetsReceived > first.packetsReceived

        val energyAdvanced: Boolean
            get() = last.totalAudioEnergy > first.totalAudioEnergy + 1e-9

        fun logLine(label: String) {
            val line =
                "ct_stats label=$label samples=${samples.size} " +
                    "bytesSent=${first.bytesSent}->${last.bytesSent} " +
                    "packetsSent=${first.packetsSent}->${last.packetsSent} " +
                    "bytesReceived=${first.bytesReceived}->${last.bytesReceived} " +
                    "packetsReceived=${first.packetsReceived}->${last.packetsReceived} " +
                    "totalAudioEnergy=${first.totalAudioEnergy}->${last.totalAudioEnergy} " +
                    "audioLevel=${last.audioLevel} ice=${last.iceConnectionState} peer=${last.peerConnectionState}"
            Log.i(LOG_TAG, line)
            println("$LOG_TAG $line")
        }
    }

    suspend fun sampleWindow(
        engine: RtcEngine,
        durationMs: Long,
        periodMs: Long = 1_000L,
    ): SampleWindow {
        val samples = mutableListOf<RtcStats>()
        val deadline = System.currentTimeMillis() + durationMs
        while (true) {
            val s = engine.stats()
            samples += s
            if (System.currentTimeMillis() >= deadline) break
            delay(periodMs)
        }
        check(samples.isNotEmpty()) { "no stats samples collected" }
        return SampleWindow(first = samples.first(), last = samples.last(), samples = samples)
    }

    fun outboundDeltaMessage(window: SampleWindow): String =
        "outbound RTP must advance over sample window: " +
            "bytesSent ${window.first.bytesSent}->${window.last.bytesSent}, " +
            "packetsSent ${window.first.packetsSent}->${window.last.packetsSent}"
}
