package com.crosstalk.translator.rtc

/**
 * Pure RTC stats reduction. Production path maps org.webrtc.RTCStatsReport into
 * [RawRtcStat] lists; unit tests feed synthetic maps without native WebRTC.
 */
data class RawRtcStat(
    val type: String,
    val id: String = "",
    val values: Map<String, Any?>,
)

object StatsSampler {
    fun sample(
        stats: Iterable<RawRtcStat>,
        iceConnectionState: String = "new",
        peerConnectionState: String = "new",
        timestampMs: Long = System.currentTimeMillis(),
    ): RtcStats {
        var bytesReceived = 0L
        var bytesSent = 0L
        var packetsReceived = 0L
        var packetsSent = 0L
        var packetsLost = 0L
        var totalAudioEnergy = 0.0
        var audioLevel = 0.0
        var jitter = 0.0
        var roundTripTime = 0.0
        var selectedCandidateType: String? = null
        var codec: String? = null
        var selectedPairId: String? = null

        val byId = stats.associateBy { it.id }

        for (s in stats) {
            when (s.type) {
                "inbound-rtp" -> {
                    if (!isAudio(s)) continue
                    bytesReceived += longVal(s, "bytesReceived")
                    packetsReceived += longVal(s, "packetsReceived")
                    packetsLost += longVal(s, "packetsLost")
                    jitter = maxOf(jitter, doubleVal(s, "jitter"))
                    val energy = doubleVal(s, "totalAudioEnergy")
                    if (energy > totalAudioEnergy) totalAudioEnergy = energy
                    val level = doubleVal(s, "audioLevel")
                    if (level > audioLevel) audioLevel = level
                    val codecId = stringVal(s, "codecId")
                    if (codec == null && codecId != null) {
                        codec = codecMime(byId[codecId])
                    }
                }
                "outbound-rtp" -> {
                    if (!isAudio(s)) continue
                    bytesSent += longVal(s, "bytesSent")
                    packetsSent += longVal(s, "packetsSent")
                    val codecId = stringVal(s, "codecId")
                    if (codec == null && codecId != null) {
                        codec = codecMime(byId[codecId])
                    }
                }
                "remote-inbound-rtp" -> {
                    if (!isAudio(s)) continue
                    val rtt = doubleVal(s, "roundTripTime")
                    if (rtt > 0) roundTripTime = rtt
                    packetsLost += longVal(s, "packetsLost")
                }
                "candidate-pair" -> {
                    val state = stringVal(s, "state")?.lowercase()
                    val nominated = boolVal(s, "nominated")
                    val selected = boolVal(s, "selected")
                    if (state == "succeeded" || nominated || selected) {
                        val rtt = doubleVal(s, "currentRoundTripTime")
                        if (rtt > 0) roundTripTime = rtt
                        selectedPairId = s.id
                        // Prefer explicitly selected/nominated pair.
                        if (selected || nominated) {
                            selectedPairId = s.id
                        }
                    }
                }
                "media-source" -> {
                    if (!isAudio(s)) continue
                    val level = doubleVal(s, "audioLevel")
                    if (level > audioLevel) audioLevel = level
                    val energy = doubleVal(s, "totalAudioEnergy")
                    if (energy > totalAudioEnergy) totalAudioEnergy = energy
                }
                "codec" -> {
                    if (codec == null) {
                        codec = codecMime(s)
                    }
                }
            }
        }

        if (selectedPairId != null) {
            val pair = byId[selectedPairId]
            val localId = pair?.let { stringVal(it, "localCandidateId") }
            val local = localId?.let { byId[it] }
            selectedCandidateType = local?.let { stringVal(it, "candidateType") }
        }

        return RtcStats(
            bytesReceived = bytesReceived,
            bytesSent = bytesSent,
            packetsReceived = packetsReceived,
            packetsSent = packetsSent,
            packetsLost = packetsLost,
            totalAudioEnergy = totalAudioEnergy,
            audioLevel = audioLevel,
            jitter = jitter,
            roundTripTime = roundTripTime,
            iceConnectionState = iceConnectionState,
            peerConnectionState = peerConnectionState,
            selectedCandidateType = selectedCandidateType,
            codec = codec,
            timestampMs = timestampMs,
        )
    }

    private fun isAudio(stat: RawRtcStat): Boolean {
        val kind = stringVal(stat, "kind") ?: stringVal(stat, "mediaType")
        // Some reports omit kind on audio-only peers; treat missing as audio.
        return kind == null || kind.equals("audio", ignoreCase = true)
    }

    private fun codecMime(stat: RawRtcStat?): String? {
        if (stat == null) return null
        val mime = stringVal(stat, "mimeType")
        if (mime != null) return mime
        val name = stringVal(stat, "name") ?: stringVal(stat, "codec")
        return name
    }

    private fun longVal(stat: RawRtcStat, key: String): Long {
        val v = stat.values[key] ?: return 0L
        return when (v) {
            is Number -> v.toLong()
            is String -> v.toLongOrNull() ?: 0L
            else -> 0L
        }
    }

    private fun doubleVal(stat: RawRtcStat, key: String): Double {
        val v = stat.values[key] ?: return 0.0
        return when (v) {
            is Number -> v.toDouble()
            is String -> v.toDoubleOrNull() ?: 0.0
            else -> 0.0
        }
    }

    private fun stringVal(stat: RawRtcStat, key: String): String? {
        val v = stat.values[key] ?: return null
        return when (v) {
            is String -> v
            else -> v.toString()
        }
    }

    private fun boolVal(stat: RawRtcStat, key: String): Boolean {
        val v = stat.values[key] ?: return false
        return when (v) {
            is Boolean -> v
            is Number -> v.toInt() != 0
            is String -> v.equals("true", ignoreCase = true) || v == "1"
            else -> false
        }
    }
}
