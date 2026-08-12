package com.crosstalk.translator.rtc

import android.content.Context
import android.media.AudioAttributes
import android.media.MediaRecorder
import android.os.Build
import com.crosstalk.translator.util.SecretRedactor
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import org.webrtc.AudioSource
import org.webrtc.AudioTrack
import org.webrtc.DataChannel
import org.webrtc.DefaultVideoDecoderFactory
import org.webrtc.DefaultVideoEncoderFactory
import org.webrtc.EglBase
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.PeerConnectionFactory
import org.webrtc.RtpReceiver
import org.webrtc.RtpTransceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import org.webrtc.audio.JavaAudioDeviceModule
import java.util.concurrent.Executors
import java.util.concurrent.ThreadFactory
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * Production libwebrtc engine (io.github.webrtc-sdk / org.webrtc).
 *
 * - PeerConnectionFactory once per engine lifecycle
 * - JavaAudioDeviceModule VOICE_COMMUNICATION + hardware AEC/NS when available
 * - Unified plan, one SEND_RECV audio transceiver + local mic track
 * - Control data channel named "control"
 * - Client offer → setLocal → send offer; trickle ICE; queue remote candidates
 * - Handle answer + server-offer renegotiation
 * - All mutations serialized on a single-thread dispatcher
 * - setMuted disables local track only
 * - Stats every 1s while connected
 * - Idempotent ordered close
 *
 * Stale-attempt fencing: each [connect] bumps [attemptId]; callbacks from older
 * attempts are ignored.
 */
final class LibWebRtcEngine(
    appContext: Context,
    private val httpClient: OkHttpClient,
    private val log: (String) -> Unit = {},
) : RtcEngine {
    private val appContext = appContext.applicationContext

    private val worker =
        Executors.newSingleThreadExecutor(
            ThreadFactory { r ->
                Thread(r, "crosstalk-rtc").apply { isDaemon = true }
            },
        )
    private val rtcDispatcher: CoroutineDispatcher = worker.asCoroutineDispatcher()
    private val scope = CoroutineScope(SupervisorJob() + rtcDispatcher)

    private val eventsFlow =
        MutableSharedFlow<RtcEvent>(
            extraBufferCapacity = 128,
            onBufferOverflow = BufferOverflow.DROP_OLDEST,
        )
    override val events: SharedFlow<RtcEvent> = eventsFlow.asSharedFlow()

    private val attemptId = AtomicLong(0)
    private val closed = AtomicBoolean(false)
    private val connecting = AtomicBoolean(false)

    private var factoryInitialized = false
    private var eglBase: EglBase? = null
    private var audioDeviceModule: JavaAudioDeviceModule? = null
    private var peerConnectionFactory: PeerConnectionFactory? = null

    private var peerConnection: PeerConnection? = null
    private var audioSource: AudioSource? = null
    private var localAudioTrack: AudioTrack? = null
    private var controlChannel: DataChannel? = null
    private var signaling: SignalingClient? = null
    private var signalingCollectJob: Job? = null
    private var statsJob: Job? = null

    private val pendingRemoteCandidates = ArrayDeque<IceCandidateInit>()
    private var remoteDescriptionSet = false
    private var muted = false
    private var iceState: String = "new"
    private var peerState: String = "new"
    private var latestStats: RtcStats = RtcStats()
    private val offerSent = AtomicBoolean(false)

    override suspend fun connect(request: RtcConnectRequest) {
        withContext(rtcDispatcher) {
            if (closed.get()) {
                throw IllegalStateException("engine closed")
            }
            // Replace any prior attempt.
            teardownSession(StopReason.Replaced, emitClosed = false)
            val id = attemptId.incrementAndGet()
            connecting.set(true)
            remoteDescriptionSet = false
            pendingRemoteCandidates.clear()
            offerSent.set(false)
            iceState = "new"
            peerState = "new"
            latestStats = RtcStats(timestampMs = System.currentTimeMillis())
            emit(RtcEvent.Connecting(id))
            emitDiag("rtc_connect attempt=$id sessionPresent=true")

            try {
                ensureFactory()
                val pc = createPeerConnection(request.iceServers, id)
                peerConnection = pc

                // Control data channel before offer (matches browser).
                val dcInit = DataChannel.Init().apply { ordered = true }
                controlChannel = pc.createDataChannel(CONTROL_CHANNEL_LABEL, dcInit)
                controlChannel?.registerObserver(
                    object : DataChannel.Observer {
                        override fun onBufferedAmountChange(previousAmount: Long) = Unit

                        override fun onStateChange() {
                            scope.launch {
                                if (!isCurrent(id)) return@launch
                                val state = controlChannel?.state()?.name ?: "unknown"
                                emit(RtcEvent.DataChannelStateChanged(CONTROL_CHANNEL_LABEL, state))
                                emitDiag("data_channel label=$CONTROL_CHANNEL_LABEL state=$state")
                            }
                        }

                        override fun onMessage(buffer: DataChannel.Buffer) {
                            scope.launch {
                                if (!isCurrent(id)) return@launch
                                val bytes = buffer.data.remaining()
                                emitDiag("data_channel_message label=$CONTROL_CHANNEL_LABEL bytes=$bytes")
                            }
                        }
                    },
                )

                // Local mic + SEND_RECV transceiver so offer has send and receive m-lines.
                val constraints =
                    MediaConstraints().apply {
                        mandatory.add(MediaConstraints.KeyValuePair("googEchoCancellation", "true"))
                        mandatory.add(MediaConstraints.KeyValuePair("googNoiseSuppression", "true"))
                        mandatory.add(MediaConstraints.KeyValuePair("googAutoGainControl", "true"))
                    }
                audioSource = peerConnectionFactory!!.createAudioSource(constraints)
                localAudioTrack =
                    peerConnectionFactory!!.createAudioTrack(LOCAL_AUDIO_TRACK_ID, audioSource).apply {
                        setEnabled(!muted)
                    }
                val transceiverInit =
                    RtpTransceiver.RtpTransceiverInit(
                        RtpTransceiver.RtpTransceiverDirection.SEND_RECV,
                        listOf(LOCAL_STREAM_ID),
                    )
                pc.addTransceiver(localAudioTrack, transceiverInit)

                // Signaling.
                val client =
                    SignalingClient(httpClient) { msg ->
                        if (isCurrent(id)) emitDiag(msg)
                    }
                signaling = client
                signalingCollectJob =
                    scope.launch {
                        client.events.collect { event ->
                            if (!isCurrent(id)) return@collect
                            handleSignalingEvent(event, id)
                        }
                    }
                client.connect(request.wsBaseUrl, request.sessionId, request.mediaTicket)

                // Create offer, set local, send when WS open.
                val offer =
                    createOffer(pc).also { sdp ->
                        setLocalDescription(pc, sdp)
                    }
                emitDiag("local_offer_created sdpBytes=${offer.description?.length ?: 0}")
                maybeSendOffer(id, offer)
                startStatsLoop(id)
                connecting.set(false)
            } catch (t: Throwable) {
                connecting.set(false)
                val msg = SecretRedactor.redact(t.message) ?: "connect failed"
                emitDiag("rtc_connect_failed error=$msg")
                emit(RtcEvent.Failed(StopReason.Error, msg))
                teardownSession(StopReason.Error, emitClosed = true)
                throw t
            }
        }
    }

    override suspend fun setMuted(muted: Boolean) {
        withContext(rtcDispatcher) {
            this@LibWebRtcEngine.muted = muted
            localAudioTrack?.setEnabled(!muted)
            emit(RtcEvent.MutedChanged(muted))
            emitDiag("mute muted=$muted")
        }
    }

    override suspend fun stats(): RtcStats =
        withContext(rtcDispatcher) {
            latestStats
        }

    override suspend fun close(reason: StopReason) {
        withContext(rtcDispatcher) {
            val firstClose = closed.compareAndSet(false, true)
            closed.set(true)
            teardownSession(reason, emitClosed = true)
            if (firstClose) {
                disposeFactory()
            }
            emitDiag("engine_closed reason=$reason first=$firstClose")
        }
    }

    /** Terminal close that also shuts the worker (for tests / process teardown). */
    fun destroy() {
        scope.launch {
            if (!closed.get()) {
                close(StopReason.UserStop)
            }
            scope.cancel()
            worker.shutdownNow()
        }
    }

    private fun isCurrent(id: Long): Boolean = !closed.get() && attemptId.get() == id

    private suspend fun handleSignalingEvent(event: SignalingClient.Event, id: Long) {
        when (event) {
            is SignalingClient.Event.StateChanged -> {
                emit(RtcEvent.SignalingStateChanged(event.state.name))
                if (event.state == SignalingClient.State.Open) {
                    // Flush offer if created before socket open.
                    val pc = peerConnection ?: return
                    val local = pc.localDescription
                    if (local != null && local.type == SessionDescription.Type.OFFER) {
                        maybeSendOffer(id, local)
                    }
                }
            }
            is SignalingClient.Event.Message -> handleRemoteMessage(event.message, id)
            is SignalingClient.Event.Failed -> {
                emit(RtcEvent.Failed(StopReason.SignalingFailed, event.message))
            }
            is SignalingClient.Event.Closed -> {
                emitDiag("signaling_session_closed code=${event.code}")
            }
        }
    }

    private suspend fun handleRemoteMessage(message: SignalingMessage, id: Long) {
        val pc = peerConnection ?: return
        when (message.type) {
            SignalingCodec.TYPE_ANSWER -> {
                val sdp = message.sdp ?: return
                setRemoteDescription(pc, SessionDescription(SessionDescription.Type.ANSWER, sdp))
                remoteDescriptionSet = true
                emit(RtcEvent.RemoteDescriptionApplied("answer"))
                flushRemoteCandidates(pc)
            }
            SignalingCodec.TYPE_OFFER -> {
                // Server renegotiation.
                val sdp = message.sdp ?: return
                setRemoteDescription(pc, SessionDescription(SessionDescription.Type.OFFER, sdp))
                remoteDescriptionSet = true
                emit(RtcEvent.RemoteDescriptionApplied("offer"))
                flushRemoteCandidates(pc)
                val answer = createAnswer(pc)
                setLocalDescription(pc, answer)
                signaling?.send(
                    SignalingMessage(
                        type = SignalingCodec.TYPE_ANSWER,
                        sdp = answer.description,
                    ),
                )
                emitDiag("renegotiation_answer_sent sdpBytes=${answer.description?.length ?: 0}")
            }
            SignalingCodec.TYPE_CANDIDATE, SignalingCodec.TYPE_ICE -> {
                val init = message.candidate ?: return
                if (!remoteDescriptionSet) {
                    pendingRemoteCandidates.addLast(init)
                    emitDiag("remote_candidate_queued size=${pendingRemoteCandidates.size}")
                } else {
                    addRemoteCandidate(pc, init)
                }
            }
        }
    }

    private fun maybeSendOffer(id: Long, offer: SessionDescription) {
        if (!isCurrent(id)) return
        if (!offerSent.compareAndSet(false, true)) return
        val client = signaling ?: return
        if (client.state() != SignalingClient.State.Open) {
            // Reset so open handler can retry.
            offerSent.set(false)
            return
        }
        val ok =
            client.send(
                SignalingMessage(
                    type = SignalingCodec.TYPE_OFFER,
                    sdp = offer.description,
                ),
            )
        if (ok) {
            emit(RtcEvent.LocalOfferSent(id))
            emitDiag("local_offer_sent attempt=$id")
        } else {
            offerSent.set(false)
        }
    }

    private fun createPeerConnection(
        iceServers: List<IceServerConfig>,
        id: Long,
    ): PeerConnection {
        val factory = peerConnectionFactory ?: error("factory missing")
        val rtcIceServers =
            iceServers.map { cfg ->
                val builder = PeerConnection.IceServer.builder(cfg.urls)
                if (!cfg.username.isNullOrEmpty()) {
                    builder.setUsername(cfg.username)
                }
                if (!cfg.credential.isNullOrEmpty()) {
                    builder.setPassword(cfg.credential)
                }
                builder.createIceServer()
            }
        val rtcConfig =
            PeerConnection.RTCConfiguration(rtcIceServers).apply {
                sdpSemantics = PeerConnection.SdpSemantics.UNIFIED_PLAN
                continualGatheringPolicy = PeerConnection.ContinualGatheringPolicy.GATHER_CONTINUALLY
                // Bundle + RTCP mux defaults are fine for audio-only.
            }

        val observer =
            object : PeerConnection.Observer {
                override fun onSignalingChange(newState: PeerConnection.SignalingState?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        emit(RtcEvent.SignalingStateChanged(newState?.name ?: "unknown"))
                    }
                }

                override fun onIceConnectionChange(newState: PeerConnection.IceConnectionState?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        iceState = newState?.name?.lowercase() ?: "unknown"
                        emit(RtcEvent.IceConnectionStateChanged(iceState))
                        emitDiag("ice_state=$iceState")
                        if (newState == PeerConnection.IceConnectionState.FAILED) {
                            emit(RtcEvent.Failed(StopReason.IceFailed, "ICE failed"))
                        }
                    }
                }

                override fun onIceConnectionReceivingChange(receiving: Boolean) = Unit

                override fun onIceGatheringChange(newState: PeerConnection.IceGatheringState?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        val name = newState?.name?.lowercase() ?: "unknown"
                        emit(RtcEvent.IceGatheringStateChanged(name))
                    }
                }

                override fun onIceCandidate(candidate: IceCandidate?) {
                    if (candidate == null) return
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        val init =
                            IceCandidateInit(
                                candidate = candidate.sdp,
                                sdpMid = candidate.sdpMid,
                                sdpMLineIndex = candidate.sdpMLineIndex,
                            )
                        val sent =
                            signaling?.send(
                                SignalingMessage(
                                    type = SignalingCodec.TYPE_CANDIDATE,
                                    candidate = init,
                                ),
                            ) == true
                        emitDiag(
                            "local_candidate sent=$sent mid=${candidate.sdpMid} " +
                                "mLine=${candidate.sdpMLineIndex}",
                        )
                    }
                }

                override fun onIceCandidatesRemoved(candidates: Array<out IceCandidate>?) = Unit

                override fun onAddStream(stream: MediaStream?) = Unit

                override fun onRemoveStream(stream: MediaStream?) = Unit

                override fun onDataChannel(dc: DataChannel?) {
                    scope.launch {
                        if (!isCurrent(id) || dc == null) return@launch
                        emitDiag("remote_data_channel label=${dc.label()}")
                    }
                }

                override fun onRenegotiationNeeded() {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        emitDiag("renegotiation_needed")
                    }
                }

                override fun onAddTrack(receiver: RtpReceiver?, mediaStreams: Array<out MediaStream>?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        val kind = receiver?.track()?.kind() ?: "unknown"
                        emit(RtcEvent.RemoteTrack(kind))
                        emitDiag("remote_track kind=$kind")
                    }
                }

                override fun onConnectionChange(newState: PeerConnection.PeerConnectionState?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        peerState = newState?.name?.lowercase() ?: "unknown"
                        emit(RtcEvent.PeerConnectionStateChanged(peerState))
                        emitDiag("peer_state=$peerState")
                        when (newState) {
                            PeerConnection.PeerConnectionState.FAILED ->
                                emit(RtcEvent.Failed(StopReason.PeerFailed, "peer connection failed"))
                            PeerConnection.PeerConnectionState.DISCONNECTED ->
                                emitDiag("peer_disconnected")
                            else -> Unit
                        }
                    }
                }

                override fun onTrack(transceiver: RtpTransceiver?) {
                    scope.launch {
                        if (!isCurrent(id)) return@launch
                        val kind = transceiver?.receiver?.track()?.kind() ?: "unknown"
                        emit(RtcEvent.RemoteTrack(kind))
                    }
                }
            }

        return factory.createPeerConnection(rtcConfig, observer)
            ?: error("createPeerConnection returned null")
    }

    private fun ensureFactory() {
        if (peerConnectionFactory != null) return
        if (!factoryInitialized) {
            val initOptions =
                PeerConnectionFactory.InitializationOptions.builder(appContext)
                    .setEnableInternalTracer(false)
                    .createInitializationOptions()
            PeerConnectionFactory.initialize(initOptions)
            factoryInitialized = true
        }

        eglBase = EglBase.create()
        val useHwAec = JavaAudioDeviceModule.isBuiltInAcousticEchoCancelerSupported()
        val useHwNs = JavaAudioDeviceModule.isBuiltInNoiseSuppressorSupported()
        val admBuilder =
            JavaAudioDeviceModule.builder(appContext)
                .setUseHardwareAcousticEchoCanceler(useHwAec)
                .setUseHardwareNoiseSuppressor(useHwNs)
                .setUseStereoInput(false)
                .setUseStereoOutput(false)
                .setAudioSource(MediaRecorder.AudioSource.VOICE_COMMUNICATION)
                .setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_VOICE_COMMUNICATION)
                        .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                        .build(),
                )
        if (Build.VERSION.SDK_INT >= 29) {
            // Prefer low-latency voice path when available.
            admBuilder.setUseLowLatency(true)
        }
        val adm = admBuilder.createAudioDeviceModule()
        audioDeviceModule = adm

        val encoderFactory =
            DefaultVideoEncoderFactory(
                eglBase!!.eglBaseContext,
                /* enableIntelVp8Encoder */ true,
                /* enableH264HighProfile */ true,
            )
        val decoderFactory = DefaultVideoDecoderFactory(eglBase!!.eglBaseContext)

        peerConnectionFactory =
            PeerConnectionFactory.builder()
                .setAudioDeviceModule(adm)
                .setVideoEncoderFactory(encoderFactory)
                .setVideoDecoderFactory(decoderFactory)
                .createPeerConnectionFactory()
        // Factory retains ADM; release the builder's local reference (WebRTC sample pattern).
        adm.release()
        emitDiag("factory_ready hwAec=$useHwAec hwNs=$useHwNs")
    }

    private fun startStatsLoop(id: Long) {
        statsJob?.cancel()
        statsJob =
            scope.launch {
                while (isActive && isCurrent(id)) {
                    collectStats(id)
                    delay(1_000)
                }
            }
    }

    private suspend fun collectStats(id: Long) {
        val pc = peerConnection ?: return
        if (!isCurrent(id)) return
        val report =
            suspendCancellableCoroutine { cont ->
                pc.getStats { rtcStatsReport ->
                    if (cont.isActive) cont.resume(rtcStatsReport)
                }
            }
        if (!isCurrent(id)) return
        val raw =
            report.statsMap.values.map { s ->
                val values = mutableMapOf<String, Any?>()
                s.members.forEach { (k, v) -> values[k] = v }
                RawRtcStat(type = s.type, id = s.id, values = values)
            }
        val sampled =
            StatsSampler.sample(
                stats = raw,
                iceConnectionState = iceState,
                peerConnectionState = peerState,
                timestampMs = System.currentTimeMillis(),
            )
        latestStats = sampled
        emit(RtcEvent.StatsUpdated(sampled))
    }

    private fun flushRemoteCandidates(pc: PeerConnection) {
        while (pendingRemoteCandidates.isNotEmpty()) {
            val c = pendingRemoteCandidates.removeFirst()
            addRemoteCandidate(pc, c)
        }
    }

    private fun addRemoteCandidate(pc: PeerConnection, init: IceCandidateInit) {
        val cand =
            IceCandidate(
                init.sdpMid,
                init.sdpMLineIndex ?: 0,
                init.candidate,
            )
        pc.addIceCandidate(cand)
        emitDiag("remote_candidate_applied mid=${init.sdpMid} mLine=${init.sdpMLineIndex}")
    }

    private suspend fun createOffer(pc: PeerConnection): SessionDescription {
        val constraints =
            MediaConstraints().apply {
                mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveAudio", "true"))
                mandatory.add(MediaConstraints.KeyValuePair("OfferToReceiveVideo", "false"))
            }
        return suspendCancellableCoroutine { cont ->
            pc.createOffer(
                object : SdpObserverAdapter() {
                    override fun onCreateSuccess(sdp: SessionDescription?) {
                        if (sdp == null) {
                            cont.resumeWithException(IllegalStateException("null offer"))
                        } else {
                            cont.resume(sdp)
                        }
                    }

                    override fun onCreateFailure(error: String?) {
                        cont.resumeWithException(IllegalStateException(error ?: "createOffer failed"))
                    }
                },
                constraints,
            )
        }
    }

    private suspend fun createAnswer(pc: PeerConnection): SessionDescription {
        val constraints = MediaConstraints()
        return suspendCancellableCoroutine { cont ->
            pc.createAnswer(
                object : SdpObserverAdapter() {
                    override fun onCreateSuccess(sdp: SessionDescription?) {
                        if (sdp == null) {
                            cont.resumeWithException(IllegalStateException("null answer"))
                        } else {
                            cont.resume(sdp)
                        }
                    }

                    override fun onCreateFailure(error: String?) {
                        cont.resumeWithException(IllegalStateException(error ?: "createAnswer failed"))
                    }
                },
                constraints,
            )
        }
    }

    private suspend fun setLocalDescription(pc: PeerConnection, sdp: SessionDescription) {
        suspendCancellableCoroutine { cont ->
            pc.setLocalDescription(
                object : SdpObserverAdapter() {
                    override fun onSetSuccess() {
                        cont.resume(Unit)
                    }

                    override fun onSetFailure(error: String?) {
                        cont.resumeWithException(IllegalStateException(error ?: "setLocalDescription failed"))
                    }
                },
                sdp,
            )
        }
    }

    private suspend fun setRemoteDescription(pc: PeerConnection, sdp: SessionDescription) {
        suspendCancellableCoroutine { cont ->
            pc.setRemoteDescription(
                object : SdpObserverAdapter() {
                    override fun onSetSuccess() {
                        cont.resume(Unit)
                    }

                    override fun onSetFailure(error: String?) {
                        cont.resumeWithException(IllegalStateException(error ?: "setRemoteDescription failed"))
                    }
                },
                sdp,
            )
        }
    }

    private fun teardownSession(reason: StopReason, emitClosed: Boolean) {
        statsJob?.cancel()
        statsJob = null
        signalingCollectJob?.cancel()
        signalingCollectJob = null

        try {
            signaling?.close()
        } catch (_: Exception) {
            // ignore
        }
        signaling = null

        try {
            controlChannel?.unregisterObserver()
            controlChannel?.close()
            controlChannel?.dispose()
        } catch (_: Exception) {
            // ignore
        }
        controlChannel = null

        try {
            localAudioTrack?.setEnabled(false)
            localAudioTrack?.dispose()
        } catch (_: Exception) {
            // ignore
        }
        localAudioTrack = null

        try {
            audioSource?.dispose()
        } catch (_: Exception) {
            // ignore
        }
        audioSource = null

        try {
            peerConnection?.close()
            peerConnection?.dispose()
        } catch (_: Exception) {
            // ignore
        }
        peerConnection = null

        pendingRemoteCandidates.clear()
        remoteDescriptionSet = false
        offerSent.set(false)
        connecting.set(false)
        iceState = "closed"
        peerState = "closed"

        if (emitClosed) {
            emit(RtcEvent.Closed(reason))
        }
    }

    private fun disposeFactory() {
        try {
            peerConnectionFactory?.dispose()
        } catch (_: Exception) {
            // ignore
        }
        peerConnectionFactory = null
        audioDeviceModule = null
        try {
            eglBase?.release()
        } catch (_: Exception) {
            // ignore
        }
        eglBase = null
    }

    private fun emit(event: RtcEvent) {
        eventsFlow.tryEmit(event)
    }

    private fun emitDiag(message: String) {
        val redacted = SecretRedactor.redact(message)
        log(redacted)
        eventsFlow.tryEmit(RtcEvent.Diagnostic(redacted))
    }

    private open class SdpObserverAdapter : SdpObserver {
        override fun onCreateSuccess(sessionDescription: SessionDescription?) = Unit

        override fun onSetSuccess() = Unit

        override fun onCreateFailure(error: String?) = Unit

        override fun onSetFailure(error: String?) = Unit
    }

    companion object {
        const val CONTROL_CHANNEL_LABEL = "control"
        private const val LOCAL_AUDIO_TRACK_ID = "ARDAMSa0"
        private const val LOCAL_STREAM_ID = "ARDAMS"
    }
}
