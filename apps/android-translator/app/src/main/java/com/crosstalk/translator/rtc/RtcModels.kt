package com.crosstalk.translator.rtc

/**
 * Production-neutral RTC domain models. No org.webrtc types leak past LibWebRtcEngine.
 */

data class IceServerConfig(
    val urls: List<String>,
    val username: String? = null,
    val credential: String? = null,
) {
    init {
        require(urls.isNotEmpty()) { "ICE server requires at least one URL" }
    }

    companion object {
        val DEFAULT_STUN: List<IceServerConfig> =
            listOf(IceServerConfig(urls = listOf("stun:stun.l.google.com:19302")))
    }
}

data class RtcConnectRequest(
    val wsBaseUrl: String,
    val sessionId: String,
    val mediaTicket: String,
    val iceServers: List<IceServerConfig> = IceServerConfig.DEFAULT_STUN,
) {
    init {
        require(wsBaseUrl.isNotBlank()) { "wsBaseUrl required" }
        require(sessionId.isNotBlank()) { "sessionId required" }
        require(mediaTicket.isNotBlank()) { "mediaTicket required" }
        require(iceServers.isNotEmpty()) { "at least one ICE server required" }
    }
}

enum class StopReason {
    UserStop,
    Replaced,
    NetworkLost,
    PermissionRevoked,
    FocusLostPermanent,
    AuthFailed,
    SignalingFailed,
    IceFailed,
    PeerFailed,
    Error,
    ProcessDeath,
}

data class RtcStats(
    val bytesReceived: Long = 0,
    val bytesSent: Long = 0,
    val packetsReceived: Long = 0,
    val packetsSent: Long = 0,
    val packetsLost: Long = 0,
    val totalAudioEnergy: Double = 0.0,
    val audioLevel: Double = 0.0,
    val jitter: Double = 0.0,
    val roundTripTime: Double = 0.0,
    val iceConnectionState: String = "new",
    val peerConnectionState: String = "new",
    val selectedCandidateType: String? = null,
    val codec: String? = null,
    val timestampMs: Long = 0,
) {
    val lossFraction: Double
        get() {
            val expected = packetsReceived + packetsLost
            if (expected <= 0L) return 0.0
            return packetsLost.toDouble() / expected.toDouble()
        }
}

sealed class RtcEvent {
    data class Diagnostic(val message: String) : RtcEvent()

    data class Connecting(val attemptId: Long) : RtcEvent()

    data class SignalingStateChanged(val state: String) : RtcEvent()

    data class IceConnectionStateChanged(val state: String) : RtcEvent()

    data class PeerConnectionStateChanged(val state: String) : RtcEvent()

    data class IceGatheringStateChanged(val state: String) : RtcEvent()

    data class LocalOfferSent(val attemptId: Long) : RtcEvent()

    data class RemoteDescriptionApplied(val type: String) : RtcEvent()

    data class RemoteTrack(val kind: String) : RtcEvent()

    data class DataChannelStateChanged(val label: String, val state: String) : RtcEvent()

    data class MutedChanged(val muted: Boolean) : RtcEvent()

    data class StatsUpdated(val stats: RtcStats) : RtcEvent()

    data class Failed(val reason: StopReason, val message: String) : RtcEvent()

    data class Closed(val reason: StopReason) : RtcEvent()
}

/**
 * Wire-format signaling message matching server `SignalMessage`
 * (offer / answer / candidate|ice, optional seq).
 */
data class SignalingMessage(
    val type: String,
    val sdp: String? = null,
    val candidate: IceCandidateInit? = null,
    val seq: Long? = null,
)

data class IceCandidateInit(
    val candidate: String? = null,
    val sdpMid: String? = null,
    val sdpMLineIndex: Int? = null,
    val usernameFragment: String? = null,
)
