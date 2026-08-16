package com.crosstalk.translator.feature.live

import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.AuthTokens
import com.crosstalk.translator.contract.BroadcastLink
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.contract.MixEntry
import com.crosstalk.translator.contract.SessionSummary
import com.crosstalk.translator.contract.SourceInfo
import com.crosstalk.translator.service.AudioServiceGateway
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.service.ServiceState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class LiveSessionViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadsHumanChannelNamesBeforeJoin() = runTest(dispatcher) {
        val api = FakeApi(
            channels = listOf(
                ChannelInfo("f1", "Floor Feed", "feed", "s1"),
                ChannelInfo("b1", "English Broadcast", "broadcast", "s1"),
            ),
        )
        val gateway = FakeGateway()
        val vm = LiveSessionViewModel(
            sessionId = "s1",
            sessionName = "Sunday Spanish",
            api = api,
            gateway = gateway,
        )
        advanceUntilIdle()
        val state = vm.uiState.value
        assertEquals("Floor Feed", state.feedName)
        assertEquals("English Broadcast", state.broadcastName)
        assertEquals("Sunday Spanish", state.sessionName)
        assertTrue(gateway.bindCalls >= 1)
    }

    @Test
    fun loadsBroadcastLinkAndTogglesQrVisibility() = runTest(dispatcher) {
        val api = FakeApi(
            broadcastLink = BroadcastLink(token = "broadcast-token", url = "/broadcast/broadcast-token"),
        )
        val vm = LiveSessionViewModel(
            sessionId = "s1",
            sessionName = "Sunday Spanish",
            apiBaseUrl = "https://crosstalk-sfu.fly.dev",
            api = api,
            gateway = FakeGateway(),
        )
        advanceUntilIdle()

        assertEquals(
            "https://crosstalk-sfu.fly.dev/broadcast/listen/s1?t=broadcast-token",
            vm.uiState.value.broadcastListenerUrl,
        )
        assertEquals(false, vm.uiState.value.qrVisible)

        vm.toggleQr()

        assertEquals(true, vm.uiState.value.qrVisible)
    }

    @Test
    fun blankBroadcastTokenKeepsQrUnavailable() = runTest(dispatcher) {
        val vm = LiveSessionViewModel(
            sessionId = "s1",
            sessionName = "Sunday Spanish",
            api = FakeApi(broadcastLink = BroadcastLink(token = "", url = "/broadcast/")),
            gateway = FakeGateway(),
            apiBaseUrl = "https://crosstalk-sfu.fly.dev",
        )
        advanceUntilIdle()

        assertEquals(null, vm.uiState.value.broadcastListenerUrl)
        assertTrue(vm.uiState.value.broadcastLinkError!!.contains("unavailable", ignoreCase = true))
    }

    @Test
    fun broadcastLinkCanRetryAfterTransientFailure() = runTest(dispatcher) {
        val api = FakeApi(
            broadcastLink = BroadcastLink("broadcast-token", "/broadcast/broadcast-token"),
            broadcastLinkFailuresBeforeSuccess = 1,
        )
        val vm = LiveSessionViewModel(
            sessionId = "s1",
            sessionName = "Sunday Spanish",
            api = api,
            gateway = FakeGateway(),
            apiBaseUrl = "https://crosstalk-sfu.fly.dev",
        )
        advanceUntilIdle()
        assertEquals(null, vm.uiState.value.broadcastListenerUrl)

        vm.retryBroadcastLink()
        advanceUntilIdle()

        assertEquals(
            "https://crosstalk-sfu.fly.dev/broadcast/listen/s1?t=broadcast-token",
            vm.uiState.value.broadcastListenerUrl,
        )
        assertEquals(null, vm.uiState.value.broadcastLinkError)
    }

    @Test
    fun routeControlsLoadSessionMixAndPersistMute() = runTest(dispatcher) {
        val api = FakeApi(
            channels = listOf(ChannelInfo("b1", "English Broadcast", "broadcast", "s1")),
            sources = listOf(SourceInfo("src1", "Booth microphone", "abc", connected = true)),
            mixes = mapOf(
                "b1" to listOf(
                    MixEntry(
                        id = "mix1",
                        channelId = "b1",
                        sourceId = "src1",
                        muted = false,
                        level = 1.0,
                    ),
                ),
            ),
        )
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", api, FakeGateway())
        advanceUntilIdle()

        vm.toggleRouteControls()
        advanceUntilIdle()

        assertEquals(true, vm.uiState.value.routeControlsVisible)
        assertEquals("Booth microphone", vm.uiState.value.routeSources.single().name)
        assertEquals(false, vm.uiState.value.mixByChannel.getValue("b1").single().muted)

        vm.setMixMuted("b1", "src1", muted = true)
        advanceUntilIdle()

        assertEquals(1, api.updateMixCalls)
        assertEquals(true, vm.uiState.value.mixByChannel.getValue("b1").single().muted)
    }

    @Test
    fun routeControlsAssignClampLevelAndRemoveSource() = runTest(dispatcher) {
        val api = FakeApi(
            channels = listOf(ChannelInfo("b1", "English Broadcast", "broadcast", "s1")),
            sources = listOf(SourceInfo("src1", "Booth microphone", "abc", connected = true)),
            mixes = mapOf("b1" to emptyList()),
        )
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", api, FakeGateway())
        advanceUntilIdle()
        vm.toggleRouteControls()
        advanceUntilIdle()

        vm.assignSource("b1", "src1")
        advanceUntilIdle()
        assertEquals("src1", vm.uiState.value.mixByChannel.getValue("b1").single().sourceId)

        vm.setMixLevel("b1", "src1", 3.0)
        advanceUntilIdle()
        assertEquals(2.0, vm.uiState.value.mixByChannel.getValue("b1").single().level, 0.0)

        vm.removeSource("b1", "src1")
        advanceUntilIdle()
        assertTrue(vm.uiState.value.mixByChannel.getValue("b1").isEmpty())
    }

    @Test
    fun failedMixSavePreservesLastServerState() = runTest(dispatcher) {
        val original = MixEntry("mix1", "b1", "src1", muted = false, level = 1.0)
        val api = FakeApi(
            channels = listOf(ChannelInfo("b1", "English Broadcast", "broadcast", "s1")),
            sources = listOf(SourceInfo("src1", "Booth microphone", "abc", connected = true)),
            mixes = mapOf("b1" to listOf(original)),
            updateMixError = ApiException.Network("down"),
        )
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", api, FakeGateway())
        advanceUntilIdle()
        vm.toggleRouteControls()
        advanceUntilIdle()

        vm.setMixMuted("b1", "src1", muted = true)
        advanceUntilIdle()

        assertEquals(false, vm.uiState.value.mixByChannel.getValue("b1").single().muted)
        assertTrue(vm.uiState.value.routeControlsError!!.contains("save", ignoreCase = true))
    }

    @Test
    fun rapidMixEditsCoalesceWithoutDroppingLatestIntent() = runTest(dispatcher) {
        val gate = CompletableDeferred<Unit>()
        val original = MixEntry("mix1", "b1", "src1", muted = false, level = 1.0)
        val api = FakeApi(
            channels = listOf(ChannelInfo("b1", "English Broadcast", "broadcast", "s1")),
            sources = listOf(SourceInfo("src1", "Booth microphone", "abc", connected = true)),
            mixes = mapOf("b1" to listOf(original)),
            updateMixGate = gate,
        )
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", api, FakeGateway())
        advanceUntilIdle()
        vm.toggleRouteControls()
        advanceUntilIdle()

        vm.setMixMuted("b1", "src1", muted = true)
        runCurrent()
        vm.setMixLevel("b1", "src1", level = 1.5)
        runCurrent()
        gate.complete(Unit)
        advanceUntilIdle()

        assertEquals(2, api.updateMixCalls)
        val saved = vm.uiState.value.mixByChannel.getValue("b1").single()
        assertEquals(true, saved.muted)
        assertEquals(1.5, saved.level, 0.0)
    }

    @Test
    fun joinRequiresResumedActivityAndMicPermission() = runTest(dispatcher) {
        val api = FakeApi(
            channels = listOf(
                ChannelInfo("f1", "Floor Feed", "feed", "s1"),
                ChannelInfo("b1", "English Broadcast", "broadcast", "s1"),
            ),
        )
        val gateway = FakeGateway()
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", api, gateway)
        advanceUntilIdle()

        vm.join()
        assertEquals(0, gateway.joinCalls)

        vm.onActivityResumed(true)
        vm.join()
        assertEquals(0, gateway.joinCalls)

        vm.onMicPermission(MicPermissionUi.Granted)
        vm.join()
        assertEquals(1, gateway.joinCalls)
        assertEquals("Sunday Spanish", gateway.lastSessionName)
        assertEquals("Floor Feed", gateway.lastFeedName)
        assertEquals("English Broadcast", gateway.lastBroadcastName)
    }

    @Test
    fun processRestoredShowsRejoinSentence() = runTest(dispatcher) {
        val gateway = FakeGateway()
        val vm = LiveSessionViewModel("s1", "Sunday Spanish", FakeApi(), gateway)
        advanceUntilIdle()
        gateway.emit(
            ServiceState(
                phase = ServicePhase.ProcessRestored,
                sessionId = "s1",
                sessionName = "Sunday Spanish",
                processRestoredMessage = "Previous session ended. Tap Rejoin.",
            ),
        )
        advanceUntilIdle()
        val state = vm.uiState.value
        assertTrue(state.requiresRejoin)
        assertTrue(
            state.statusSentence.contains("Rejoin", ignoreCase = true) ||
                state.statusSentence.contains("ended", ignoreCase = true),
        )
    }

    @Test
    fun statusSentenceCoversConnectedAndMuted() {
        val local = LiveSessionUiState(micPermission = MicPermissionUi.Granted)
        assertEquals(
            "Connected. Listening and speaking.",
            LiveSessionViewModel.buildStatusSentence(
                local,
                ServiceState(phase = ServicePhase.Connected),
            ),
        )
        assertEquals(
            "Connected. Microphone muted.",
            LiveSessionViewModel.buildStatusSentence(
                local,
                ServiceState(phase = ServicePhase.Muted),
            ),
        )
        assertEquals(
            "Waiting for network…",
            LiveSessionViewModel.buildStatusSentence(
                local,
                ServiceState(phase = ServicePhase.WaitingForNetwork),
            ),
        )
    }

    @Test
    fun stopAndMuteForwardToGateway() = runTest(dispatcher) {
        val gateway = FakeGateway()
        val vm = LiveSessionViewModel("s1", "S", FakeApi(), gateway)
        advanceUntilIdle()
        vm.setMuted(true)
        vm.stop()
        assertEquals(true, gateway.lastMuted)
        assertEquals(1, gateway.stopCalls)
    }

    @Test
    fun channelLoadNetworkErrorSurfaces() = runTest(dispatcher) {
        val api = FakeApi(channelsError = ApiException.Network("down"))
        val vm = LiveSessionViewModel("s1", "S", api, FakeGateway())
        advanceUntilIdle()
        assertTrue(vm.uiState.value.channelsError!!.contains("network", ignoreCase = true))
    }

    @Test
    fun unknownChannelWhenNameMissing() = runTest(dispatcher) {
        val api = FakeApi(
            channels = listOf(
                ChannelInfo("f1", "", "feed", "s1"),
                ChannelInfo("b1", "English Broadcast", "broadcast", "s1"),
            ),
        )
        val vm = LiveSessionViewModel("s1", "S", api, FakeGateway())
        advanceUntilIdle()
        assertEquals("Unknown channel", vm.uiState.value.feedName)
        assertEquals("English Broadcast", vm.uiState.value.broadcastName)
    }

    private class FakeGateway : AudioServiceGateway {
        private val _state = MutableStateFlow(ServiceState.Idle)
        override val state: StateFlow<ServiceState> = _state.asStateFlow()
        var bindCalls = 0
        var joinCalls = 0
        var stopCalls = 0
        var lastMuted: Boolean? = null
        var lastSessionName: String? = null
        var lastFeedName: String? = null
        var lastBroadcastName: String? = null

        fun emit(s: ServiceState) {
            _state.value = s
        }

        override fun join(
            sessionId: String,
            sessionName: String,
            feedName: String,
            broadcastName: String,
            feedIds: List<String>,
            broadcastIds: List<String>,
        ) {
            joinCalls++
            lastSessionName = sessionName
            lastFeedName = feedName
            lastBroadcastName = broadcastName
            _state.value = ServiceState(
                phase = ServicePhase.Preparing,
                sessionId = sessionId,
                sessionName = sessionName,
                feedName = feedName,
                broadcastName = broadcastName,
            )
        }

        override fun setMuted(muted: Boolean) {
            lastMuted = muted
        }

        override fun stop() {
            stopCalls++
            _state.value = ServiceState(phase = ServicePhase.Stopped, wasExplicitlyStopped = true)
        }

        override fun bind() {
            bindCalls++
        }

        override fun unbind() = Unit
    }

    private class FakeApi(
        private val channels: List<ChannelInfo> = emptyList(),
        private val channelsError: Exception? = null,
        private val broadcastLink: BroadcastLink? = null,
        private val sources: List<SourceInfo> = emptyList(),
        private val mixes: Map<String, List<MixEntry>> = emptyMap(),
        private val updateMixError: Exception? = null,
        private val broadcastLinkFailuresBeforeSuccess: Int = 0,
        private val updateMixGate: CompletableDeferred<Unit>? = null,
    ) : CrossTalkApi {
        var updateMixCalls: Int = 0
        private var broadcastLinkCalls: Int = 0
        override suspend fun login(username: String, password: String): AuthTokens = error("unused")
        override suspend fun refresh(refreshToken: String): AuthTokens = error("unused")
        override suspend fun logout(refreshToken: String) = Unit
        override suspend fun listSessions(): List<SessionSummary> = emptyList()
        override suspend fun getSession(sessionId: String): SessionSummary = error("unused")
        override suspend fun listChannels(sessionId: String): List<ChannelInfo> {
            channelsError?.let { throw it }
            return channels
        }

        override suspend fun getBroadcastLink(sessionId: String): BroadcastLink {
            broadcastLinkCalls++
            if (broadcastLinkCalls <= broadcastLinkFailuresBeforeSuccess) {
                throw ApiException.Network("temporary")
            }
            return broadcastLink ?: error("unused")
        }

        override suspend fun listSources(sessionId: String): List<SourceInfo> = sources

        override suspend fun getMix(sessionId: String, channelId: String): List<MixEntry> =
            mixes[channelId].orEmpty()

        override suspend fun updateMix(
            sessionId: String,
            channelId: String,
            entries: List<MixEntry>,
        ): List<MixEntry> {
            updateMixCalls++
            updateMixError?.let { throw it }
            updateMixGate?.await()
            return entries
        }

        override suspend fun mintMediaTicket(sessionId: String): MediaTicket =
            error("unused")
    }
}
