package com.crosstalk.translator.rtc

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference

/**
 * Test-only RtcEngine. Lives under src/test so it is never packaged into the APK.
 *
 * Supports stale-attempt fencing via [attemptId]: callbacks recorded against an
 * older attempt are dropped when [applyStaleCallback] is used after a newer connect.
 */
class FakeRtcEngine : RtcEngine {
    private val eventsFlow =
        MutableSharedFlow<RtcEvent>(
            extraBufferCapacity = 256,
            onBufferOverflow = kotlinx.coroutines.channels.BufferOverflow.DROP_OLDEST,
        )
    override val events: SharedFlow<RtcEvent> = eventsFlow.asSharedFlow()

    private val closed = AtomicBoolean(false)
    private val muted = AtomicBoolean(false)
    private val attemptId = AtomicLong(0)
    private val lastRequest = AtomicReference<RtcConnectRequest?>(null)
    private val lastCloseReason = AtomicReference<StopReason?>(null)
    private val closeCount = AtomicLong(0)
    private val statsRef = AtomicReference(RtcStats())
    private val receivePathAlive = AtomicBoolean(true)

    /** Exposed for contract assertions. */
    fun currentAttemptId(): Long = attemptId.get()

    fun lastConnectRequest(): RtcConnectRequest? = lastRequest.get()

    fun lastCloseReason(): StopReason? = lastCloseReason.get()

    fun closeCount(): Long = closeCount.get()

    fun isMuted(): Boolean = muted.get()

    fun isReceivePathAlive(): Boolean = receivePathAlive.get()

    fun isClosed(): Boolean = closed.get()

    fun setStats(stats: RtcStats) {
        statsRef.set(stats)
        eventsFlow.tryEmit(RtcEvent.StatsUpdated(stats))
    }

    /**
     * Simulate an async callback bound to [forAttempt]. No-ops when the attempt
     * is stale (fencing).
     */
    fun applyStaleCallback(forAttempt: Long, event: RtcEvent): Boolean {
        if (closed.get()) return false
        if (attemptId.get() != forAttempt) return false
        eventsFlow.tryEmit(event)
        return true
    }

    fun emit(event: RtcEvent) {
        if (closed.get() && event !is RtcEvent.Closed) return
        eventsFlow.tryEmit(event)
    }

    override suspend fun connect(request: RtcConnectRequest) {
        if (closed.get()) {
            // Allow reconnect after close for contract tests that re-open lifecycle.
            closed.set(false)
            receivePathAlive.set(true)
        }
        val id = attemptId.incrementAndGet()
        lastRequest.set(request)
        eventsFlow.tryEmit(RtcEvent.Connecting(id))
        eventsFlow.tryEmit(RtcEvent.SignalingStateChanged("Open"))
        eventsFlow.tryEmit(RtcEvent.LocalOfferSent(id))
        eventsFlow.tryEmit(RtcEvent.RemoteDescriptionApplied("answer"))
        eventsFlow.tryEmit(RtcEvent.IceConnectionStateChanged("connected"))
        eventsFlow.tryEmit(RtcEvent.PeerConnectionStateChanged("connected"))
        eventsFlow.tryEmit(RtcEvent.RemoteTrack("audio"))
        statsRef.set(
            RtcStats(
                bytesReceived = 100,
                bytesSent = 80,
                packetsReceived = 10,
                packetsSent = 8,
                iceConnectionState = "connected",
                peerConnectionState = "connected",
                timestampMs = System.currentTimeMillis(),
            ),
        )
        eventsFlow.tryEmit(RtcEvent.StatsUpdated(statsRef.get()))
    }

    override suspend fun setMuted(muted: Boolean) {
        this.muted.set(muted)
        // Mute never tears down receive path.
        receivePathAlive.set(true)
        eventsFlow.tryEmit(RtcEvent.MutedChanged(muted))
    }

    override suspend fun stats(): RtcStats = statsRef.get()

    override suspend fun close(reason: StopReason) {
        closeCount.incrementAndGet()
        lastCloseReason.set(reason)
        if (closed.getAndSet(true)) {
            // Idempotent: still emit Closed so observers can reconcile.
            eventsFlow.tryEmit(RtcEvent.Closed(reason))
            return
        }
        receivePathAlive.set(false)
        eventsFlow.tryEmit(RtcEvent.Closed(reason))
    }
}
