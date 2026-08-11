package com.crosstalk.translator.rtc

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class StatsSamplerTest {
    @Test
    fun samplesInboundOutboundAudioAndRtt() {
        val stats =
            listOf(
                RawRtcStat(
                    type = "inbound-rtp",
                    id = "in1",
                    values =
                        mapOf(
                            "kind" to "audio",
                            "bytesReceived" to 12_000L,
                            "packetsReceived" to 100L,
                            "packetsLost" to 4L,
                            "jitter" to 0.012,
                            "totalAudioEnergy" to 1.5,
                            "audioLevel" to 0.25,
                            "codecId" to "codec1",
                        ),
                ),
                RawRtcStat(
                    type = "outbound-rtp",
                    id = "out1",
                    values =
                        mapOf(
                            "kind" to "audio",
                            "bytesSent" to 8_000L,
                            "packetsSent" to 90L,
                            "codecId" to "codec1",
                        ),
                ),
                RawRtcStat(
                    type = "candidate-pair",
                    id = "pair1",
                    values =
                        mapOf(
                            "state" to "succeeded",
                            "nominated" to true,
                            "selected" to true,
                            "currentRoundTripTime" to 0.042,
                            "localCandidateId" to "local1",
                        ),
                ),
                RawRtcStat(
                    type = "local-candidate",
                    id = "local1",
                    values = mapOf("candidateType" to "srflx"),
                ),
                RawRtcStat(
                    type = "codec",
                    id = "codec1",
                    values = mapOf("mimeType" to "audio/opus"),
                ),
            )

        val sample =
            StatsSampler.sample(
                stats = stats,
                iceConnectionState = "connected",
                peerConnectionState = "connected",
                timestampMs = 1_700_000_000_000L,
            )

        assertEquals(12_000L, sample.bytesReceived)
        assertEquals(8_000L, sample.bytesSent)
        assertEquals(100L, sample.packetsReceived)
        assertEquals(90L, sample.packetsSent)
        assertEquals(4L, sample.packetsLost)
        assertEquals(1.5, sample.totalAudioEnergy, 1e-9)
        assertEquals(0.25, sample.audioLevel, 1e-9)
        assertEquals(0.012, sample.jitter, 1e-9)
        assertEquals(0.042, sample.roundTripTime, 1e-9)
        assertEquals("connected", sample.iceConnectionState)
        assertEquals("connected", sample.peerConnectionState)
        assertEquals("srflx", sample.selectedCandidateType)
        assertEquals("audio/opus", sample.codec)
        assertEquals(1_700_000_000_000L, sample.timestampMs)
        assertTrue(sample.lossFraction > 0.0)
    }

    @Test
    fun ignoresVideoRtpWhenKindPresent() {
        val stats =
            listOf(
                RawRtcStat(
                    type = "inbound-rtp",
                    id = "vin",
                    values =
                        mapOf(
                            "kind" to "video",
                            "bytesReceived" to 999_999L,
                            "packetsReceived" to 999L,
                        ),
                ),
                RawRtcStat(
                    type = "inbound-rtp",
                    id = "ain",
                    values =
                        mapOf(
                            "kind" to "audio",
                            "bytesReceived" to 10L,
                            "packetsReceived" to 1L,
                        ),
                ),
            )
        val sample = StatsSampler.sample(stats)
        assertEquals(10L, sample.bytesReceived)
        assertEquals(1L, sample.packetsReceived)
    }

    @Test
    fun emptyReportYieldsZeros() {
        val sample = StatsSampler.sample(emptyList())
        assertEquals(0L, sample.bytesReceived)
        assertEquals(0L, sample.bytesSent)
        assertEquals(0.0, sample.totalAudioEnergy, 0.0)
        assertNull(sample.selectedCandidateType)
        assertEquals(0.0, sample.lossFraction, 0.0)
    }

    @Test
    fun mediaSourceContributesAudioLevel() {
        val stats =
            listOf(
                RawRtcStat(
                    type = "media-source",
                    id = "src",
                    values =
                        mapOf(
                            "kind" to "audio",
                            "audioLevel" to 0.8,
                            "totalAudioEnergy" to 3.3,
                        ),
                ),
            )
        val sample = StatsSampler.sample(stats)
        assertEquals(0.8, sample.audioLevel, 1e-9)
        assertEquals(3.3, sample.totalAudioEnergy, 1e-9)
    }
}
