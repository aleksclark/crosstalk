package com.crosstalk.translator.service

import com.crosstalk.translator.rtc.RtcStats

/**
 * Immutable service snapshot observed by Compose via binder / [AudioServiceGateway].
 */
enum class ServicePhase {
    Idle,
    Preparing,
    Minting,
    Signaling,
    IceChecking,
    Connected,
    Muted,
    ReconnectScheduled,
    WaitingForNetwork,
    Failed,
    Stopped,
    ProcessRestored,
}

data class ServiceState(
    val phase: ServicePhase = ServicePhase.Idle,
    /** Monotonic generation for fencing stale callbacks after Stop / replace. */
    val generation: Long = 0L,
    val sessionId: String? = null,
    val sessionName: String? = null,
    val feedName: String? = null,
    val broadcastName: String? = null,
    val feedIds: List<String> = emptyList(),
    val broadcastIds: List<String> = emptyList(),
    /** User-requested mute (manual). Transient focus mute does not flip this. */
    val micMuted: Boolean = false,
    /** Effective capture mute including transient focus loss. */
    val captureSuppressed: Boolean = false,
    val inputLevel: Float = 0f,
    val outputLevel: Float = 0f,
    val attemptCount: Int = 0,
    val nextRetryAtEpochMs: Long? = null,
    val errorReason: String? = null,
    val stats: RtcStats? = null,
    val wasExplicitlyStopped: Boolean = false,
    val processRestoredMessage: String? = null,
    val connectedSinceEpochMs: Long? = null,
    val userRequestedLive: Boolean = false,
) {
    val isLiveOrConnecting: Boolean
        get() = when (phase) {
            ServicePhase.Preparing,
            ServicePhase.Minting,
            ServicePhase.Signaling,
            ServicePhase.IceChecking,
            ServicePhase.Connected,
            ServicePhase.Muted,
            ServicePhase.ReconnectScheduled,
            ServicePhase.WaitingForNetwork,
            -> true
            else -> false
        }

    val notificationStatusLabel: String
        get() = when (phase) {
            ServicePhase.Connected -> "Connected"
            ServicePhase.Muted -> "Muted"
            ServicePhase.ReconnectScheduled -> "Reconnecting"
            ServicePhase.WaitingForNetwork -> "Waiting for network"
            ServicePhase.Preparing, ServicePhase.Minting, ServicePhase.Signaling, ServicePhase.IceChecking -> "Connecting"
            ServicePhase.Failed -> "Failed"
            ServicePhase.Stopped -> "Stopped"
            ServicePhase.ProcessRestored -> "Session ended"
            ServicePhase.Idle -> "Idle"
        }

    companion object {
        val Idle = ServiceState()
    }
}
