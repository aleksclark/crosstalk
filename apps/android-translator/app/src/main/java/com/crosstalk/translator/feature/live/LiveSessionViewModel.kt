package com.crosstalk.translator.feature.live

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.MixEntry
import com.crosstalk.translator.contract.SourceInfo
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
    val broadcastListenerUrl: String? = null,
    val broadcastLinkLoading: Boolean = false,
    val broadcastLinkError: String? = null,
    val qrVisible: Boolean = false,
    val routeControlsVisible: Boolean = false,
    val routeControlsLoading: Boolean = false,
    val routeControlsLoaded: Boolean = false,
    val routeControlsError: String? = null,
    val routeChannels: List<ChannelInfo> = emptyList(),
    val routeSources: List<SourceInfo> = emptyList(),
    val mixByChannel: Map<String, List<MixEntry>> = emptyMap(),
    val savingMixChannelIds: Set<String> = emptySet(),
    /** True when process-restored requires explicit Rejoin. */
    val requiresRejoin: Boolean = false,
)

class LiveSessionViewModel(
    private val sessionId: String,
    private val sessionName: String,
    private val api: CrossTalkApi,
    private val gateway: AudioServiceGateway,
    private val apiBaseUrl: String = "",
) : ViewModel() {
    private val _uiState = MutableStateFlow(
        LiveSessionUiState(
            sessionId = sessionId,
            sessionName = sessionName.ifBlank { sessionId },
        ).withDerived(),
    )
    val uiState: StateFlow<LiveSessionUiState> = _uiState.asStateFlow()
    private val confirmedMixByChannel = mutableMapOf<String, List<MixEntry>>()
    private val pendingMixByChannel = mutableMapOf<String, List<MixEntry>>()
    private val writingMixChannelIds = mutableSetOf<String>()

    init {
        gateway.bind()
        viewModelScope.launch {
            gateway.state.collect { service ->
                _uiState.update { it.copy(service = service).withDerived() }
            }
        }
        loadChannels()
        loadBroadcastLink()
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
                        routeChannels = channels,
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

    fun toggleQr() {
        _uiState.update { state ->
            if (state.broadcastListenerUrl == null) state else state.copy(qrVisible = !state.qrVisible)
        }
    }

    fun retryBroadcastLink() {
        if (_uiState.value.broadcastLinkLoading) return
        loadBroadcastLink()
    }

    fun toggleRouteControls() {
        val nextVisible = !_uiState.value.routeControlsVisible
        _uiState.update { it.copy(routeControlsVisible = nextVisible) }
        if (nextVisible && !_uiState.value.routeControlsLoaded && !_uiState.value.routeControlsLoading) {
            loadRouteControls()
        }
    }

    fun retryRouteControls() {
        if (_uiState.value.routeControlsLoading) return
        loadRouteControls()
    }

    private fun loadRouteControls() {
        _uiState.update {
            it.copy(
                routeControlsLoading = true,
                routeControlsError = null,
            )
        }
        viewModelScope.launch {
            try {
                val channels = _uiState.value.routeChannels.ifEmpty { api.listChannels(sessionId) }
                val sources = api.listSources(sessionId)
                val mixes = buildMap {
                    channels.forEach { channel ->
                        put(channel.id, api.getMix(sessionId, channel.id))
                    }
                }
                confirmedMixByChannel.clear()
                confirmedMixByChannel.putAll(mixes)
                pendingMixByChannel.clear()
                _uiState.update {
                    it.copy(
                        routeControlsLoading = false,
                        routeControlsLoaded = true,
                        routeControlsError = null,
                        routeChannels = channels,
                        routeSources = sources,
                        mixByChannel = mixes,
                    )
                }
            } catch (e: ApiException.Forbidden) {
                _uiState.update {
                    it.copy(
                        routeControlsLoading = false,
                        routeControlsError = "You do not have access to session channel controls.",
                    )
                }
            } catch (e: ApiException.Unauthorized) {
                _uiState.update {
                    it.copy(
                        routeControlsLoading = false,
                        routeControlsError = "Session expired. Sign in again.",
                    )
                }
            } catch (e: ApiException.Network) {
                _uiState.update {
                    it.copy(
                        routeControlsLoading = false,
                        routeControlsError = "Unable to load channel controls. Check network.",
                    )
                }
            } catch (_: Exception) {
                _uiState.update {
                    it.copy(
                        routeControlsLoading = false,
                        routeControlsError = "Session channel controls are unavailable.",
                    )
                }
            }
        }
    }

    fun assignSource(channelId: String, sourceId: String) {
        val current = _uiState.value.mixByChannel[channelId].orEmpty()
        if (current.any { it.sourceId == sourceId }) return
        persistMix(
            channelId,
            current + MixEntry(
                id = "",
                channelId = channelId,
                sourceId = sourceId,
                muted = false,
                level = 1.0,
            ),
        )
    }

    fun removeSource(channelId: String, sourceId: String) {
        val current = _uiState.value.mixByChannel[channelId].orEmpty()
        persistMix(channelId, current.filterNot { it.sourceId == sourceId })
    }

    fun setMixMuted(channelId: String, sourceId: String, muted: Boolean) {
        val current = _uiState.value.mixByChannel[channelId].orEmpty()
        persistMix(
            channelId,
            current.map { entry ->
                if (entry.sourceId == sourceId) entry.copy(muted = muted) else entry
            },
        )
    }

    fun setMixLevel(channelId: String, sourceId: String, level: Double) {
        val current = _uiState.value.mixByChannel[channelId].orEmpty()
        persistMix(
            channelId,
            current.map { entry ->
                if (entry.sourceId == sourceId) {
                    entry.copy(level = level.coerceIn(0.0, 2.0))
                } else {
                    entry
                }
            },
        )
    }

    private fun persistMix(channelId: String, desired: List<MixEntry>) {
        pendingMixByChannel[channelId] = desired
        _uiState.update {
            it.copy(
                mixByChannel = it.mixByChannel + (channelId to desired),
                savingMixChannelIds = it.savingMixChannelIds + channelId,
                routeControlsError = null,
            )
        }
        if (!writingMixChannelIds.add(channelId)) return

        viewModelScope.launch {
            try {
                while (true) {
                    val next = pendingMixByChannel.remove(channelId) ?: break
                    try {
                        val saved = api.updateMix(sessionId, channelId, next)
                        confirmedMixByChannel[channelId] = saved
                        if (channelId !in pendingMixByChannel) {
                            _uiState.update {
                                it.copy(mixByChannel = it.mixByChannel + (channelId to saved))
                            }
                        }
                    } catch (_: Exception) {
                        if (channelId !in pendingMixByChannel) {
                            val confirmed = confirmedMixByChannel[channelId].orEmpty()
                            _uiState.update {
                                it.copy(
                                    mixByChannel = it.mixByChannel + (channelId to confirmed),
                                    routeControlsError = "Could not save channel mix. Try again.",
                                )
                            }
                            break
                        }
                    }
                }
            } finally {
                writingMixChannelIds.remove(channelId)
                _uiState.update {
                    it.copy(
                        savingMixChannelIds = it.savingMixChannelIds - channelId,
                    )
                }
            }
        }
    }

    private fun loadBroadcastLink() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    broadcastLinkLoading = true,
                    broadcastLinkError = null,
                    qrVisible = false,
                )
            }
            try {
                val link = api.getBroadcastLink(sessionId)
                require(link.token.isNotBlank()) { "Broadcast token is unavailable" }
                require(apiBaseUrl.isNotBlank()) { "Server URL is unavailable" }
                val listenerUrl = buildBroadcastListenerUrl(apiBaseUrl, sessionId, link.token)
                _uiState.update {
                    it.copy(
                        broadcastLinkLoading = false,
                        broadcastListenerUrl = listenerUrl,
                        broadcastLinkError = null,
                    )
                }
            } catch (e: ApiException.Unauthorized) {
                _uiState.update {
                    it.copy(
                        broadcastLinkLoading = false,
                        broadcastLinkError = "Session expired. Sign in again.",
                    )
                }
            } catch (e: ApiException.Network) {
                _uiState.update {
                    it.copy(
                        broadcastLinkLoading = false,
                        broadcastLinkError = "Unable to load broadcast link. Check network.",
                    )
                }
            } catch (_: Exception) {
                _uiState.update {
                    it.copy(
                        broadcastLinkLoading = false,
                        broadcastLinkError = "Broadcast link unavailable.",
                    )
                }
            }
        }
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
        // Do not pre-select every session channel ID. Empty selectors preserve
        // server-derived translator ticket scope (feed listen / broadcast produce).
        // Display names come from the session channel list for UX; service re-resolves
        // against the minted ticket after mint.
        gateway.join(
            sessionId = s.sessionId,
            sessionName = s.sessionName,
            feedName = feed,
            broadcastName = broadcast,
            feedIds = emptyList(),
            broadcastIds = emptyList(),
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
        fun buildBroadcastListenerUrl(
            apiBaseUrl: String,
            sessionId: String,
            token: String,
        ): String {
            val base = apiBaseUrl.trim().trimEnd('/')
            val encodedSession = java.net.URLEncoder.encode(sessionId, Charsets.UTF_8.name())
            val encodedToken = java.net.URLEncoder.encode(token, Charsets.UTF_8.name())
            return "$base/broadcast/listen/$encodedSession?t=$encodedToken"
        }

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
        private val apiBaseUrl: String,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(LiveSessionViewModel::class.java))
            return LiveSessionViewModel(sessionId, sessionName, api, gateway, apiBaseUrl) as T
        }
    }
}
