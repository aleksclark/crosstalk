package com.crosstalk.translator.service

import android.Manifest
import android.app.Service
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.Binder
import android.os.IBinder
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.crosstalk.translator.CrossTalkApplication
import com.crosstalk.translator.app.AppContainer
import com.crosstalk.translator.audio.AudioFocusController
import com.crosstalk.translator.audio.AudioRouteController
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.LastSession
import com.crosstalk.translator.rtc.RtcConnectRequest
import com.crosstalk.translator.rtc.RtcEngine
import com.crosstalk.translator.rtc.RtcEvent
import com.crosstalk.translator.rtc.StopReason
import com.crosstalk.translator.util.Clock
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.atomic.AtomicReference

/**
 * Foreground owner of mint + RTC + focus/route/wake/network for live translation.
 *
 * Start sequence: onStartCommand → startForeground (mic|mediaPlayback) immediately,
 * then orchestration. START_NOT_STICKY. Stop always wins over reconnect.
 */
class TranslatorAudioService : Service() {

    inner class LocalBinder : Binder() {
        val state: StateFlow<ServiceState>
            get() = _state.asStateFlow()

        fun dispatch(command: ServiceCommand) {
            this@TranslatorAudioService.dispatch(command)
        }

        fun setStateListener(listener: ((ServiceState) -> Unit)?) {
            stateListener.set(listener)
        }

        fun service(): TranslatorAudioService = this@TranslatorAudioService

        /**
         * Most recent successfully minted media ticket token (opaque).
         * Used by instrumented golden tests to prove reconnect mints a fresh ticket
         * (`token != previous`). Never log this value.
         */
        fun lastMediaTicketToken(): String? = lastMediaTicketToken.get()

        /** Snapshot of reducer-mapped RTC stats for binder/debug probes. */
        fun statsSnapshot(): com.crosstalk.translator.rtc.RtcStats? = _state.value.stats

        /**
         * Single-line, secret-free stats dump for golden harness / logcat polling.
         * Format is stable for `run-device-golden.sh` parsers.
         */
        fun debugStatsLine(): String {
            val s = _state.value
            val st = s.stats
            return buildString {
                append("ct_stats")
                append(" phase=").append(s.phase.name)
                append(" gen=").append(s.generation)
                append(" live=").append(s.userRequestedLive)
                if (st != null) {
                    append(" bytesSent=").append(st.bytesSent)
                    append(" bytesReceived=").append(st.bytesReceived)
                    append(" packetsSent=").append(st.packetsSent)
                    append(" packetsReceived=").append(st.packetsReceived)
                    append(" packetsLost=").append(st.packetsLost)
                    append(" totalAudioEnergy=").append(st.totalAudioEnergy)
                    append(" audioLevel=").append(st.audioLevel)
                    append(" ice=").append(st.iceConnectionState)
                    append(" peer=").append(st.peerConnectionState)
                    append(" ts=").append(st.timestampMs)
                } else {
                    append(" stats=null")
                }
                val ticket = lastMediaTicketToken.get()
                if (ticket != null) {
                    // Fingerprint only — never the full token.
                    append(" ticketFp=").append(ticketFingerprint(ticket))
                }
            }
        }
    }

    private val binder = LocalBinder()
    private val stateListener = AtomicReference<((ServiceState) -> Unit)?>(null)
    /** Last minted media ticket token for reconnect freshness assertions (never logged raw). */
    private val lastMediaTicketToken = AtomicReference<String?>(null)

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val orchestrateMutex = Mutex()

    private val _state = MutableStateFlow(ServiceState.Idle)

    private lateinit var container: AppContainer
    private lateinit var notification: ServiceNotification
    private lateinit var wakeLease: WakeLease
    private lateinit var networkMonitor: NetworkMonitor
    private lateinit var focusController: AudioFocusController
    private lateinit var routeController: AudioRouteController
    private lateinit var lastSessionStore: LastSessionStore
    private lateinit var clock: Clock
    private lateinit var reconnectPolicy: ReconnectPolicy
    private lateinit var rtcFactory: () -> RtcEngine

    private var rtcEngine: RtcEngine? = null
    private var rtcEventsJob: Job? = null
    private var reconnectJob: Job? = null
    private var watchdogJob: Job? = null
    private var iceGraceJob: Job? = null
    private var reconnectBudget = ReconnectPolicy.Budget()
    private var foregroundStarted = false
    private var destroying = false

    override fun onCreate() {
        super.onCreate()
        val app = application as CrossTalkApplication
        container = app.container
        clock = container.clock
        reconnectPolicy = ReconnectPolicy()
        notification = ServiceNotification(this)
        notification.ensureChannel()
        wakeLease = WakeLease(this, clock = clock)
        lastSessionStore = container.lastSessionStore
        rtcFactory = container.rtcEngineFactory
        routeController = AudioRouteController(this)
        focusController =
            AudioFocusController(
                this,
                listener =
                    object : AudioFocusController.Listener {
                        override fun onTransientFocusLoss() {
                            dispatchFenced(ServiceEvent.Fenced.TransientFocusLost)
                            scope.launch { applyEffectiveMute() }
                        }

                        override fun onFocusGain() {
                            dispatchFenced(ServiceEvent.Fenced.FocusRegained)
                            scope.launch { applyEffectiveMute() }
                        }

                        override fun onPermanentFocusLoss() {
                            scope.launch { handlePermanentFocusLoss() }
                        }
                    },
            )
        networkMonitor =
            NetworkMonitor(this) { validated ->
                scope.launch {
                    if (validated) {
                        onNetworkValidated()
                    } else {
                        onNetworkLost()
                    }
                }
            }
        networkMonitor.start()
        scope.launch { maybeRestoreProcessDeathUx() }
    }

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // IMMEDIATELY promote to foreground with BOTH type bits before network/RTC.
        ensureForeground()

        when (intent?.action) {
            ACTION_JOIN -> {
                val join = intent.toJoinCommand()
                if (join != null) {
                    dispatch(join)
                }
            }
            ACTION_MUTE -> dispatch(ServiceCommand.SetMuted(true))
            ACTION_UNMUTE -> dispatch(ServiceCommand.SetMuted(false))
            ACTION_STOP -> dispatch(ServiceCommand.Stop)
            ACTION_SET_MUTED -> {
                val muted = intent.getBooleanExtra(EXTRA_MUTED, false)
                dispatch(ServiceCommand.SetMuted(muted))
            }
        }
        return START_NOT_STICKY
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        // Keep service alive; do not Stop.
        super.onTaskRemoved(rootIntent)
    }

    override fun onDestroy() {
        destroying = true
        scope.launch {
            performFullCleanup(reason = StopReason.ProcessDeath, userStop = false)
            scope.cancel()
        }
        // Synchronous best-effort cleanup for process teardown races.
        runCatching {
            reconnectJob?.cancel()
            watchdogJob?.cancel()
            iceGraceJob?.cancel()
            rtcEventsJob?.cancel()
            networkMonitor.stop()
            wakeLease.release()
            focusController.abandon()
            routeController.clear()
            if (foregroundStarted) {
                stopForeground(STOP_FOREGROUND_REMOVE)
                foregroundStarted = false
            }
            notification.cancel()
        }
        super.onDestroy()
    }

    fun dispatch(command: ServiceCommand) {
        val previous = _state.value
        val next = ServiceReducer.reduce(previous, ServiceEvent.Command(command))
        publish(next)
        when (command) {
            is ServiceCommand.Join -> scope.launch { onJoin(next.generation) }
            is ServiceCommand.SetMuted -> scope.launch { onSetMuted(command.muted, next.generation) }
            ServiceCommand.Stop -> scope.launch { onStop() }
        }
    }

    private fun dispatchFenced(body: ServiceEvent.Fenced, generation: Long = _state.value.generation) {
        val previous = _state.value
        val next =
            ServiceReducer.reduce(
                previous,
                ServiceEvent.GenerationFenced(generation = generation, body = body),
            )
        if (next !== previous && next != previous) {
            publish(next)
        } else if (next != previous) {
            publish(next)
        }
    }

    private fun publish(state: ServiceState) {
        _state.value = state
        stateListener.get()?.invoke(state)
        if (foregroundStarted && state.userRequestedLive) {
            notification.notify(state)
        }
    }

    private fun ensureForeground() {
        if (foregroundStarted) {
            // Refresh notification snapshot.
            notification.notify(_state.value)
            return
        }
        notification.ensureChannel()
        val notif = notification.build(_state.value)
        val typeMask =
            ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE or
                ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK
        try {
            ServiceCompat.startForeground(
                this,
                ServiceNotification.NOTIFICATION_ID,
                notif,
                typeMask,
            )
            foregroundStarted = true
        } catch (e: Exception) {
            // Surface promotion failure as terminal failed state.
            val failed =
                ServiceReducer.reduce(
                    _state.value,
                    ServiceEvent.GenerationFenced(
                        generation = _state.value.generation,
                        body =
                            ServiceEvent.Fenced.TerminalFailed(
                                reason = "Foreground start failed: ${e.javaClass.simpleName}",
                            ),
                    ),
                )
            publish(failed)
            stopSelf()
        }
    }

    private suspend fun onJoin(generation: Long) {
        orchestrateMutex.withLock {
            if (_state.value.generation != generation) return
            cancelReconnectLocked()
            reconnectBudget = reconnectPolicy.onUserLiveStarted(clock.nowEpochMs())
            wakeLease.acquireOrRenew()
            startWatchdog()
            persistLastSession(explicitStop = false)
            beginConnectAttempt(generation, isReconnect = false)
        }
    }

    private suspend fun onSetMuted(muted: Boolean, generation: Long) {
        if (_state.value.generation != generation && _state.value.userRequestedLive.not()) return
        val engine = rtcEngine
        if (engine != null) {
            val effective = muted || _state.value.captureSuppressed && !_state.value.micMuted
            // micMuted already set by reducer; apply track state for effective capture.
            runCatching {
                engine.setMuted(_state.value.captureSuppressed || muted)
            }
        }
        if (foregroundStarted) {
            notification.notify(_state.value)
        }
    }

    private suspend fun applyEffectiveMute() {
        val state = _state.value
        val engine = rtcEngine ?: return
        runCatching { engine.setMuted(state.captureSuppressed || state.micMuted) }
        if (foregroundStarted) notification.notify(state)
    }

    private suspend fun onStop() {
        orchestrateMutex.withLock {
            performFullCleanup(reason = StopReason.UserStop, userStop = true)
            persistLastSession(explicitStop = true)
            if (foregroundStarted) {
                stopForeground(STOP_FOREGROUND_REMOVE)
                foregroundStarted = false
            }
            notification.cancel()
            stopSelf()
        }
    }

    private suspend fun handlePermanentFocusLoss() {
        orchestrateMutex.withLock {
            val gen = _state.value.generation
            dispatchFenced(ServiceEvent.Fenced.PermanentFocusLost, gen)
            performFullCleanup(reason = StopReason.FocusLostPermanent, userStop = false)
            if (foregroundStarted) {
                stopForeground(STOP_FOREGROUND_REMOVE)
                foregroundStarted = false
            }
            notification.cancel()
            stopSelf()
        }
    }

    private suspend fun beginConnectAttempt(generation: Long, isReconnect: Boolean) {
        if (_state.value.generation != generation || !_state.value.userRequestedLive) return

        if (!hasMicPermission()) {
            dispatchFenced(ServiceEvent.Fenced.PermissionRevoked, generation)
            performFullCleanup(reason = StopReason.PermissionRevoked, userStop = false)
            stopAfterTerminal()
            return
        }

        if (!networkMonitor.isValidated()) {
            dispatchFenced(ServiceEvent.Fenced.NetworkLost, generation)
            reconnectBudget = reconnectPolicy.onNetworkLost(reconnectBudget)
            return
        }

        dispatchFenced(ServiceEvent.Fenced.Preparing, generation)
        wakeLease.acquireOrRenew()

        // Tear down prior peer/socket before minting a fresh ticket.
        closeEngine(StopReason.Replaced)

        if (!focusController.request()) {
            dispatchFenced(
                ServiceEvent.Fenced.TerminalFailed("Audio focus denied"),
                generation,
            )
            performFullCleanup(reason = StopReason.FocusLostPermanent, userStop = false)
            stopAfterTerminal()
            return
        }
        routeController.applyPreferredRoute()

        val joinState = _state.value
        val sessionId = joinState.sessionId
        if (sessionId.isNullOrBlank()) {
            dispatchFenced(ServiceEvent.Fenced.TerminalFailed("Missing session"), generation)
            stopAfterTerminal()
            return
        }

        dispatchFenced(ServiceEvent.Fenced.Minting, generation)
        val authRepository = container.authRepository
        val ticket =
            try {
                authRepository.requireAccessToken()
                authRepository.mintMediaTicket(sessionId)
            } catch (e: ApiException.Unauthorized) {
                dispatchFenced(ServiceEvent.Fenced.AuthFailed, generation)
                performFullCleanup(reason = StopReason.AuthFailed, userStop = false)
                stopAfterTerminal()
                return
            } catch (e: ApiException.Forbidden) {
                dispatchFenced(ServiceEvent.Fenced.AssignmentForbidden, generation)
                performFullCleanup(reason = StopReason.AuthFailed, userStop = false)
                stopAfterTerminal()
                return
            } catch (e: ApiException.Client) {
                if (e.statusCode == 403) {
                    dispatchFenced(ServiceEvent.Fenced.AssignmentForbidden, generation)
                    performFullCleanup(reason = StopReason.AuthFailed, userStop = false)
                    stopAfterTerminal()
                    return
                }
                handleRetryableFailure(
                    generation = generation,
                    kind = ReconnectPolicy.FailureKind.Transport,
                    message = e.message ?: "Mint failed",
                )
                return
            } catch (e: ApiException.Network) {
                handleRetryableFailure(
                    generation = generation,
                    kind = ReconnectPolicy.FailureKind.Network,
                    message = e.message ?: "Network error",
                )
                return
            } catch (e: Exception) {
                handleRetryableFailure(
                    generation = generation,
                    kind = ReconnectPolicy.FailureKind.Transport,
                    message = e.message ?: "Mint failed",
                )
                return
            }

        if (_state.value.generation != generation || !_state.value.userRequestedLive) return

        when (
            val v =
                MediaTicketValidator.validate(
                    ticket = ticket,
                    expectedSessionId = sessionId,
                    requestedFeedIds = joinState.feedIds,
                    requestedBroadcastIds = joinState.broadcastIds,
                    clock = clock,
                )
        ) {
            is MediaTicketValidator.Result.Rejected -> {
                dispatchFenced(ServiceEvent.Fenced.MalformedContract, generation)
                // Malformed is non-retryable.
                performFullCleanup(reason = StopReason.Error, userStop = false)
                stopAfterTerminal()
                return
            }
            MediaTicketValidator.Result.Ok -> Unit
        }

        // Record after validation so reconnect freshness checks only see accepted tickets.
        lastMediaTicketToken.set(ticket.token)

        dispatchFenced(ServiceEvent.Fenced.Signaling, generation)
        val engine = rtcFactory()
        rtcEngine = engine
        attachRtcEvents(engine, generation)

        try {
            engine.connect(
                RtcConnectRequest(
                    wsBaseUrl = container.wsBaseUrl(),
                    sessionId = sessionId,
                    mediaTicket = ticket.token,
                ),
            )
            // Apply mute preference after connect.
            engine.setMuted(_state.value.micMuted || _state.value.captureSuppressed)
        } catch (e: Exception) {
            handleRetryableFailure(
                generation = generation,
                kind = ReconnectPolicy.FailureKind.Signaling,
                message = e.message ?: "Connect failed",
            )
        }
    }

    private fun attachRtcEvents(engine: RtcEngine, generation: Long) {
        rtcEventsJob?.cancel()
        rtcEventsJob =
            scope.launch {
                engine.events.collect { event ->
                    if (_state.value.generation != generation) return@collect
                    when (event) {
                        is RtcEvent.Connecting ->
                            dispatchFenced(ServiceEvent.Fenced.Signaling, generation)
                        is RtcEvent.LocalOfferSent,
                        is RtcEvent.RemoteDescriptionApplied,
                        ->
                            dispatchFenced(ServiceEvent.Fenced.Signaling, generation)
                        is RtcEvent.IceConnectionStateChanged -> {
                            when (event.state.lowercase()) {
                                "checking", "connected", "completed" -> {
                                    if (event.state.equals("checking", ignoreCase = true)) {
                                        dispatchFenced(ServiceEvent.Fenced.IceChecking, generation)
                                    } else {
                                        iceGraceJob?.cancel()
                                        onRtcConnected(generation)
                                    }
                                }
                                "disconnected" -> {
                                    iceGraceJob?.cancel()
                                    iceGraceJob =
                                        scope.launch {
                                            delay(ReconnectPolicy.ICE_DISCONNECT_GRACE_MS)
                                            if (_state.value.generation != generation) return@launch
                                            if (!_state.value.userRequestedLive) return@launch
                                            handleRetryableFailure(
                                                generation = generation,
                                                kind = ReconnectPolicy.FailureKind.Ice,
                                                message = "ICE disconnected",
                                            )
                                        }
                                }
                                "failed", "closed" -> {
                                    iceGraceJob?.cancel()
                                    handleRetryableFailure(
                                        generation = generation,
                                        kind = ReconnectPolicy.FailureKind.Ice,
                                        message = "ICE ${event.state}",
                                    )
                                }
                            }
                        }
                        is RtcEvent.PeerConnectionStateChanged -> {
                            when (event.state.lowercase()) {
                                "connected" -> onRtcConnected(generation)
                                "failed" ->
                                    handleRetryableFailure(
                                        generation = generation,
                                        kind = ReconnectPolicy.FailureKind.Transport,
                                        message = "Peer failed",
                                    )
                                "closed" -> {
                                    // Closed after intentional teardown — ignore if not live.
                                }
                            }
                        }
                        is RtcEvent.StatsUpdated ->
                            dispatchFenced(ServiceEvent.Fenced.Stats(event.stats), generation)
                        is RtcEvent.MutedChanged -> {
                            // User mute preference is owned by ServiceCommand.SetMuted.
                            // RTC echo must not clobber manual mute after races with connect.
                        }
                        is RtcEvent.Failed -> {
                            val kind =
                                when (event.reason) {
                                    StopReason.AuthFailed -> ReconnectPolicy.FailureKind.Auth
                                    StopReason.PermissionRevoked ->
                                        ReconnectPolicy.FailureKind.MicPermission
                                    StopReason.FocusLostPermanent ->
                                        ReconnectPolicy.FailureKind.PermanentFocusLoss
                                    StopReason.SignalingFailed ->
                                        ReconnectPolicy.FailureKind.Signaling
                                    StopReason.IceFailed -> ReconnectPolicy.FailureKind.Ice
                                    StopReason.NetworkLost -> ReconnectPolicy.FailureKind.Network
                                    else -> ReconnectPolicy.FailureKind.Transport
                                }
                            if (!reconnectPolicy.isRetryable(kind)) {
                                dispatchFenced(
                                    ServiceEvent.Fenced.TerminalFailed(event.message),
                                    generation,
                                )
                                performFullCleanup(reason = event.reason, userStop = false)
                                stopAfterTerminal()
                            } else {
                                handleRetryableFailure(generation, kind, event.message)
                            }
                        }
                        is RtcEvent.Closed -> {
                            // Intentional.
                        }
                        else -> Unit
                    }
                }
            }
    }

    private fun onRtcConnected(generation: Long) {
        if (_state.value.generation != generation) return
        dispatchFenced(ServiceEvent.Fenced.Connected, generation)
        reconnectBudget = reconnectPolicy.onConnected(reconnectBudget, clock.nowEpochMs())
        wakeLease.acquireOrRenew()
        if (foregroundStarted) notification.notify(_state.value)
    }

    private suspend fun handleRetryableFailure(
        generation: Long,
        kind: ReconnectPolicy.FailureKind,
        message: String,
    ) {
        if (_state.value.generation != generation || !_state.value.userRequestedLive) return
        closeEngine(StopReason.Error)
        dispatchFenced(
            ServiceEvent.Fenced.TransportFailed(reason = message, retryable = true),
            generation,
        )

        val decision =
            reconnectPolicy.decide(
                budget = reconnectBudget,
                kind = kind,
                nowEpochMs = clock.nowEpochMs(),
                networkValidated = networkMonitor.isValidated(),
            )
        when (decision) {
            is ReconnectPolicy.Decision.ScheduleRetry -> {
                reconnectBudget = decision.next
                val nextAt = clock.nowEpochMs() + decision.delayMs
                dispatchFenced(
                    ServiceEvent.Fenced.RetryScheduled(
                        attemptCount = decision.next.attemptCount,
                        nextRetryAtEpochMs = nextAt,
                    ),
                    generation,
                )
                reconnectJob?.cancel()
                reconnectJob =
                    scope.launch {
                        delay(decision.delayMs)
                        if (_state.value.generation != generation) return@launch
                        if (!_state.value.userRequestedLive) return@launch
                        orchestrateMutex.withLock {
                            beginConnectAttempt(generation, isReconnect = true)
                        }
                    }
            }
            ReconnectPolicy.Decision.WaitForNetwork -> {
                reconnectBudget = reconnectPolicy.onNetworkLost(reconnectBudget)
                dispatchFenced(ServiceEvent.Fenced.NetworkLost, generation)
            }
            is ReconnectPolicy.Decision.GiveUp -> {
                dispatchFenced(ServiceEvent.Fenced.AttemptsExhausted, generation)
                performFullCleanup(reason = StopReason.Error, userStop = false)
                stopAfterTerminal()
            }
            ReconnectPolicy.Decision.DoNotRetry -> {
                // Stop path.
            }
        }
    }

    private suspend fun onNetworkLost() {
        val state = _state.value
        if (!state.userRequestedLive) return
        val gen = state.generation
        reconnectBudget = reconnectPolicy.onNetworkLost(reconnectBudget)
        cancelReconnectLocked()
        closeEngine(StopReason.NetworkLost)
        dispatchFenced(ServiceEvent.Fenced.NetworkLost, gen)
    }

    private suspend fun onNetworkValidated() {
        val state = _state.value
        if (!state.userRequestedLive) return
        val gen = state.generation
        reconnectBudget = reconnectPolicy.onNetworkValidated(reconnectBudget)
        dispatchFenced(ServiceEvent.Fenced.NetworkValidated, gen)
        if (state.phase == ServicePhase.WaitingForNetwork ||
            _state.value.phase == ServicePhase.ReconnectScheduled ||
            _state.value.phase == ServicePhase.WaitingForNetwork
        ) {
            // Resume with a fresh ticket + peer.
            orchestrateMutex.withLock {
                if (_state.value.generation != gen || !_state.value.userRequestedLive) return
                beginConnectAttempt(gen, isReconnect = true)
            }
        }
    }

    private fun startWatchdog() {
        watchdogJob?.cancel()
        watchdogJob =
            scope.launch {
                while (isActive) {
                    delay(WATCHDOG_PERIOD_MS)
                    val state = _state.value
                    if (!state.userRequestedLive) continue
                    // Renew wake while live.
                    wakeLease.acquireOrRenew()
                    // Permission revocation check.
                    if (!hasMicPermission()) {
                        val gen = state.generation
                        dispatchFenced(ServiceEvent.Fenced.PermissionRevoked, gen)
                        orchestrateMutex.withLock {
                            performFullCleanup(
                                reason = StopReason.PermissionRevoked,
                                userStop = false,
                            )
                            stopAfterTerminal()
                        }
                        return@launch
                    }
                    // Stable-connected attempt reset.
                    val nextBudget =
                        reconnectPolicy.onStableConnectedCheck(
                            reconnectBudget,
                            clock.nowEpochMs(),
                        )
                    if (nextBudget.attemptCount == 0 && reconnectBudget.attemptCount != 0) {
                        reconnectBudget = nextBudget
                        dispatchFenced(ServiceEvent.Fenced.StableConnectedReset, state.generation)
                    } else {
                        reconnectBudget = nextBudget
                    }
                    if (foregroundStarted) {
                        notification.notify(_state.value)
                    }
                }
            }
    }

    private suspend fun performFullCleanup(reason: StopReason, userStop: Boolean) {
        cancelReconnectLocked()
        watchdogJob?.cancel()
        watchdogJob = null
        iceGraceJob?.cancel()
        iceGraceJob = null
        closeEngine(reason)
        focusController.abandon()
        routeController.clear()
        wakeLease.release()
        if (userStop) {
            // Clear ticket after explicit stop so a later Join is not compared against a dead peer.
            lastMediaTicketToken.set(null)
        }
    }

    private suspend fun closeEngine(reason: StopReason) {
        rtcEventsJob?.cancel()
        rtcEventsJob = null
        val engine = rtcEngine
        rtcEngine = null
        if (engine != null) {
            runCatching { engine.close(reason) }
        }
    }

    private fun cancelReconnectLocked() {
        reconnectJob?.cancel()
        reconnectJob = null
    }

    private fun stopAfterTerminal() {
        if (destroying) return
        if (foregroundStarted) {
            // Keep a final failed notification briefly is optional; plan: remove on stop,
            // but failed state may still want visibility — update then demote.
            notification.notify(_state.value)
            stopForeground(STOP_FOREGROUND_DETACH)
            foregroundStarted = false
        }
        stopSelf()
    }

    private fun hasMicPermission(): Boolean =
        ContextCompat.checkSelfPermission(this, Manifest.permission.RECORD_AUDIO) ==
            PackageManager.PERMISSION_GRANTED

    private suspend fun persistLastSession(explicitStop: Boolean) {
        val s = _state.value
        val id = s.sessionId ?: return
        val name = s.sessionName ?: return
        lastSessionStore.save(
            LastSession(
                sessionId = id,
                sessionName = name,
                feedChannelName = s.feedName,
                broadcastChannelName = s.broadcastName,
                wasExplicitlyStopped = explicitStop,
            ),
        )
    }

    private suspend fun maybeRestoreProcessDeathUx() {
        val last = lastSessionStore.read() ?: return
        if (last.wasExplicitlyStopped) return
        // Process was restored without an active live service → show rejoin UX.
        if (_state.value.phase == ServicePhase.Idle) {
            val restored =
                ServiceState(
                    phase = ServicePhase.ProcessRestored,
                    sessionId = last.sessionId,
                    sessionName = last.sessionName,
                    feedName = last.feedChannelName,
                    broadcastName = last.broadcastChannelName,
                    userRequestedLive = false,
                    processRestoredMessage = "Session ended when Android stopped the app",
                    wasExplicitlyStopped = false,
                )
            publish(restored)
        }
    }

    private fun Intent.toJoinCommand(): ServiceCommand.Join? {
        val sessionId = getStringExtra(EXTRA_SESSION_ID) ?: return null
        val sessionName = getStringExtra(EXTRA_SESSION_NAME) ?: return null
        val feedName = getStringExtra(EXTRA_FEED_NAME).orEmpty()
        val broadcastName = getStringExtra(EXTRA_BROADCAST_NAME).orEmpty()
        val feedIds = getStringArrayListExtra(EXTRA_FEED_IDS)?.toList().orEmpty()
        val broadcastIds = getStringArrayListExtra(EXTRA_BROADCAST_IDS)?.toList().orEmpty()
        return ServiceCommand.Join(
            sessionId = sessionId,
            sessionName = sessionName,
            feedName = feedName,
            broadcastName = broadcastName,
            feedIds = feedIds,
            broadcastIds = broadcastIds,
        )
    }

    companion object {
        const val ACTION_JOIN = "com.crosstalk.translator.action.JOIN"
        const val ACTION_MUTE = "com.crosstalk.translator.action.MUTE"
        const val ACTION_UNMUTE = "com.crosstalk.translator.action.UNMUTE"
        const val ACTION_STOP = "com.crosstalk.translator.action.STOP"
        const val ACTION_SET_MUTED = "com.crosstalk.translator.action.SET_MUTED"

        const val EXTRA_SESSION_ID = "session_id"
        const val EXTRA_SESSION_NAME = "session_name"
        const val EXTRA_FEED_NAME = "feed_name"
        const val EXTRA_BROADCAST_NAME = "broadcast_name"
        const val EXTRA_FEED_IDS = "feed_ids"
        const val EXTRA_BROADCAST_IDS = "broadcast_ids"
        const val EXTRA_MUTED = "muted"

        private const val WATCHDOG_PERIOD_MS = 15_000L

        /** Stable short fingerprint for log/golden polling (never the raw ticket). */
        fun ticketFingerprint(token: String): String {
            var h = 0x811c9dc5.toInt()
            for (c in token) {
                h = h xor c.code
                h *= 0x01000193
            }
            val unsigned = h.toLong() and 0xffffffffL
            return unsigned.toString(16).padStart(8, '0')
        }
    }
}
