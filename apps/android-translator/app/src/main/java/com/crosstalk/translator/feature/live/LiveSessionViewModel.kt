package com.crosstalk.translator.feature.live

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.service.AudioServiceGateway
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.service.ServiceState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class MicPermissionUi {
    NotRequested,
    Rationale,
    Denied,
    PermanentlyDenied,
    Granted,
    RevokedLive,
}

enum class NotificationPermissionUi {
    NotRequested,
    Granted,
    Denied,
}

data class LiveSessionUiState(
    val sessionId: String = "",
    val sessionName: String = "",
    val feedName: String = "",
    val broadcastName: String = "",
    val feedIds: List<String> = emptyList(),
    val broadcastIds: List<String> = emptyList(),
    val channelsLoading: Boolean = false,
    val channelsError: String? = null,
    val service: ServiceState = ServiceState.Idle,
    val statusSentence: String = "Ready to join.",
    val micPermission: MicPermissionUi = MicPermissionUi.NotRequested,
    val notificationPermission: NotificationPermissionUi = NotificationPermissionUi.NotRequested,
    val activityResumed: Boolean = false,
    val diagnosticsExpanded: Boolean = false,
    /** True when process-restored requires explicit Rejoin. */
    val requiresRejoin: Boolean = false,
)

class LiveSessionViewModel(
    private val sessionId: String,
    private val sessionName: String,
    private val api: CrossTalkApi,
    private val gateway: AudioServiceGateway,
) : ViewModel() {
    private val _uiState = MutableStateFlow(
        LiveSessionUiState(
            sessionId = sessionId,
            sessionName = sessionName.ifBlank { sessionId },
        ).withDerived(),
    )
    val uiState: StateFlow<LiveSessionUiState> = _uiState.asStateFlow()

    init {
        gateway.bind()
        viewModelScope.launch {
            gateway.state.collect { service ->
                _uiState.update { it.copy(service = service).withDerived() }
            }
        }
        loadChannels()
    }

    override fun onCleared() {
        // Do not unbind here — service may outlive this screen while live.
        super.onCleared()
    }

    fun loadChannels() {
        viewModelScope.launch {
            _uiState.update { it.copy(channelsLoading = true, channelsError = null).withDerived() }
            try {
                val channels = api.listChannels(sessionId)
                val resolved = resolveChannelNames(channels)
                _uiState.update {
                    it.copy(
                        channelsLoading = false,
                        feedName = resolved.feedName,
                        broadcastName = resolved.broadcastName,
                        feedIds = resolved.feedIds,
                        broadcastIds = resolved.broadcastIds,
                        channelsError = resolved.degradedMessage,
                    ).withDerived()
                }
            } catch (e: ApiException.Unauthorized) {
                _uiState.update {
                    it.copy(
                        channelsLoading = false,
                        channelsError = "Session expired. Sign in again.",
                    ).withDerived()
                }
            } catch (e: ApiException.Network) {
                _uiState.update {
                    it.copy(
                        channelsLoading = false,
                        channelsError = "Unable to load channels. Check network.",
                    ).withDerived()
                }
            } catch (e: Exception) {
                _uiState.update {
                    it.copy(
                        channelsLoading = false,
                        channelsError = e.message?.takeIf { m -> m.isNotBlank() }
                            ?: "Unable to load channels",
                    ).withDerived()
                }
            }
        }
    }

    fun onActivityResumed(resumed: Boolean) {
        _uiState.update { it.copy(activityResumed = resumed).withDerived() }
    }

    fun onMicPermission(state: MicPermissionUi) {
        _uiState.update { it.copy(micPermission = state).withDerived() }
    }

    fun onNotificationPermission(state: NotificationPermissionUi) {
        _uiState.update { it.copy(notificationPermission = state).withDerived() }
    }

    fun toggleDiagnostics() {
        _uiState.update { it.copy(diagnosticsExpanded = !it.diagnosticsExpanded) }
    }

    /**
     * Join only when the activity is resumed and RECORD_AUDIO is granted.
     * Process-restored never auto-captures — caller must use explicit Rejoin.
     */
    fun join() {
        val s = _uiState.value
        if (!s.activityResumed) return
        if (s.micPermission != MicPermissionUi.Granted) return
        if (s.channelsLoading) return
        val feed = s.feedName.ifBlank { "Unknown channel" }
        val broadcast = s.broadcastName.ifBlank { "Unknown channel" }
        gateway.join(
            sessionId = s.sessionId,
            sessionName = s.sessionName,
            feedName = feed,
            broadcastName = broadcast,
            feedIds = s.feedIds,
            broadcastIds = s.broadcastIds,
        )
    }

    fun setMuted(muted: Boolean) {
        gateway.setMuted(muted)
    }

    fun stop() {
        gateway.stop()
    }

    fun rejoin() {
        // Explicit user action after process restore.
        join()
    }

    private data class ResolvedChannels(
        val feedName: String,
        val broadcastName: String,
        val feedIds: List<String>,
        val broadcastIds: List<String>,
        val degradedMessage: String?,
    )

    private fun resolveChannelNames(channels: List<ChannelInfo>): ResolvedChannels {
        val feeds = channels.filter { it.type.equals("feed", ignoreCase = true) }
        val broadcasts = channels.filter { it.type.equals("broadcast", ignoreCase = true) }
        val feed = feeds.firstOrNull()
        val broadcast = broadcasts.firstOrNull()
        var degraded: String? = null
        val feedName = when {
            feed != null && feed.name.isNotBlank() -> feed.name
            feed != null -> {
                degraded = "Unknown channel"
                "Unknown channel"
            }
            else -> {
                degraded = "No feed channel assigned"
                "Unknown channel"
            }
        }
        val broadcastName = when {
            broadcast != null && broadcast.name.isNotBlank() -> broadcast.name
            broadcast != null -> {
                degraded = "Unknown channel"
                "Unknown channel"
            }
            else -> {
                degraded = degraded ?: "No broadcast channel assigned"
                "Unknown channel"
            }
        }
        return ResolvedChannels(
            feedName = feedName,
            broadcastName = broadcastName,
            feedIds = feeds.map { it.id },
            broadcastIds = broadcasts.map { it.id },
            degradedMessage = degraded,
        )
    }

    private fun LiveSessionUiState.withDerived(): LiveSessionUiState {
        return copy(
            statusSentence = buildStatusSentence(this, service),
            requiresRejoin = service.phase == ServicePhase.ProcessRestored,
        )
    }

    companion object {
        fun buildStatusSentence(local: LiveSessionUiState, service: ServiceState): String {
            if (local.micPermission == MicPermissionUi.RevokedLive) {
                return "Microphone permission was revoked. Grant access and rejoin."
            }
            if (local.micPermission == MicPermissionUi.PermanentlyDenied) {
                return "Microphone access is blocked. Open Settings to allow it."
            }
            if (local.micPermission == MicPermissionUi.Denied) {
                return "Microphone permission is required to join."
            }
            if (local.micPermission == MicPermissionUi.Rationale) {
                return "This app needs the microphone to send your translation."
            }
            return when (service.phase) {
                ServicePhase.Idle -> "Ready to join."
                ServicePhase.Preparing -> "Preparing live audio…"
                ServicePhase.Minting -> "Minting media ticket…"
                ServicePhase.Signaling -> "Connecting signaling…"
                ServicePhase.IceChecking -> "Checking network path…"
                ServicePhase.Connected -> "Connected. Listening and speaking."
                ServicePhase.Muted -> "Connected. Microphone muted."
                ServicePhase.ReconnectScheduled -> "Connection lost. Reconnecting…"
                ServicePhase.WaitingForNetwork -> "Waiting for network…"
                ServicePhase.Failed -> service.errorReason?.takeIf { it.isNotBlank() }
                    ?: "Connection failed."
                ServicePhase.Stopped -> "Stopped."
                ServicePhase.ProcessRestored ->
                    service.processRestoredMessage?.takeIf { it.isNotBlank() }
                        ?: "Previous session ended. Tap Rejoin to continue."
            }
        }
    }

    class Factory(
        private val sessionId: String,
        private val sessionName: String,
        private val api: CrossTalkApi,
        private val gateway: AudioServiceGateway,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(LiveSessionViewModel::class.java))
            return LiveSessionViewModel(sessionId, sessionName, api, gateway) as T
        }
    }
}
