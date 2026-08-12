package com.crosstalk.translator.service

import com.crosstalk.translator.rtc.RtcStats

/**
 * Pure service state machine. Stop always wins; generation fencing drops stale events.
 */
object ServiceReducer {

    fun reduce(state: ServiceState, event: ServiceEvent): ServiceState =
        when (event) {
            is ServiceEvent.Command -> reduceCommand(state, event.command)
            is ServiceEvent.GenerationFenced -> {
                if (event.generation != state.generation) state
                else reduceFenced(state, event.body)
            }
        }

    private fun reduceCommand(state: ServiceState, command: ServiceCommand): ServiceState =
        when (command) {
            is ServiceCommand.Join -> {
                state.copy(
                    phase = ServicePhase.Preparing,
                    generation = state.generation + 1L,
                    sessionId = command.sessionId,
                    sessionName = command.sessionName,
                    feedName = command.feedName,
                    broadcastName = command.broadcastName,
                    feedIds = command.feedIds,
                    broadcastIds = command.broadcastIds,
                    micMuted = false,
                    captureSuppressed = false,
                    inputLevel = 0f,
                    outputLevel = 0f,
                    attemptCount = 0,
                    nextRetryAtEpochMs = null,
                    errorReason = null,
                    stats = null,
                    wasExplicitlyStopped = false,
                    processRestoredMessage = null,
                    connectedSinceEpochMs = null,
                    userRequestedLive = true,
                )
            }

            is ServiceCommand.SetMuted -> {
                if (!state.userRequestedLive && !state.phase.isLivePhase()) {
                    state
                } else {
                    // Preserve focus-only suppression bit: if capture was suppressed while
                    // not manually muted, that means transient focus loss.
                    val focusSuppressed = state.captureSuppressed && !state.micMuted
                    val nextPhase = when {
                        state.phase == ServicePhase.Connected && command.muted -> ServicePhase.Muted
                        state.phase == ServicePhase.Muted && !command.muted -> ServicePhase.Connected
                        else -> state.phase
                    }
                    state.copy(
                        micMuted = command.muted,
                        captureSuppressed = command.muted || focusSuppressed,
                        phase = nextPhase,
                    )
                }
            }

            ServiceCommand.Stop -> stop(state)
        }

    private fun reduceFenced(state: ServiceState, body: ServiceEvent.Fenced): ServiceState {
        // After Stop/terminal, ignore operational fenced events (generation already bumped
        // on stop/terminal; this is a belt-and-suspenders guard).
        if (!state.userRequestedLive &&
            body !is ServiceEvent.Fenced.ProcessRestored &&
            state.phase != ServicePhase.ProcessRestored
        ) {
            if (state.phase == ServicePhase.Stopped ||
                state.phase == ServicePhase.Idle ||
                state.phase == ServicePhase.Failed
            ) {
                return state
            }
        }

        return when (body) {
            ServiceEvent.Fenced.Preparing ->
                state.copy(phase = ServicePhase.Preparing, errorReason = null)

            ServiceEvent.Fenced.Minting ->
                state.copy(phase = ServicePhase.Minting)

            ServiceEvent.Fenced.Signaling ->
                state.copy(phase = ServicePhase.Signaling)

            ServiceEvent.Fenced.IceChecking ->
                state.copy(phase = ServicePhase.IceChecking)

            ServiceEvent.Fenced.Connected ->
                state.copy(
                    phase = if (state.micMuted) ServicePhase.Muted else ServicePhase.Connected,
                    nextRetryAtEpochMs = null,
                    errorReason = null,
                    connectedSinceEpochMs = state.connectedSinceEpochMs
                        ?: System.currentTimeMillis(),
                )

            is ServiceEvent.Fenced.Stats ->
                state.copy(
                    stats = body.stats,
                    inputLevel = body.stats.audioLevel.toFloat().coerceIn(0f, 1f),
                    outputLevel = meterFromEnergy(body.stats.totalAudioEnergy),
                )

            is ServiceEvent.Fenced.Levels ->
                state.copy(
                    inputLevel = body.input.coerceIn(0f, 1f),
                    outputLevel = body.output.coerceIn(0f, 1f),
                )

            is ServiceEvent.Fenced.MutedApplied -> {
                val muted = body.muted
                val focusSuppressed = state.captureSuppressed && !state.micMuted
                state.copy(
                    micMuted = muted,
                    captureSuppressed = muted || focusSuppressed,
                    phase = when (state.phase) {
                        ServicePhase.Connected, ServicePhase.Muted ->
                            if (muted) ServicePhase.Muted else ServicePhase.Connected
                        else -> state.phase
                    },
                )
            }

            ServiceEvent.Fenced.TransientFocusLost ->
                state.copy(captureSuppressed = true)

            ServiceEvent.Fenced.FocusRegained ->
                // Preserve manual mute; clear only focus-driven suppression.
                state.copy(captureSuppressed = state.micMuted)

            ServiceEvent.Fenced.PermanentFocusLost ->
                terminal(state, "Audio focus permanently lost")

            ServiceEvent.Fenced.NetworkLost ->
                if (!state.userRequestedLive) {
                    state
                } else {
                    state.copy(
                        phase = ServicePhase.WaitingForNetwork,
                        nextRetryAtEpochMs = null,
                        connectedSinceEpochMs = null,
                    )
                }

            ServiceEvent.Fenced.NetworkValidated ->
                if (state.phase == ServicePhase.WaitingForNetwork && state.userRequestedLive) {
                    state.copy(phase = ServicePhase.ReconnectScheduled)
                } else {
                    state
                }

            is ServiceEvent.Fenced.RetryScheduled ->
                if (!state.userRequestedLive) {
                    state
                } else {
                    state.copy(
                        phase = ServicePhase.ReconnectScheduled,
                        attemptCount = body.attemptCount,
                        nextRetryAtEpochMs = body.nextRetryAtEpochMs,
                        connectedSinceEpochMs = null,
                    )
                }

            is ServiceEvent.Fenced.TransportFailed ->
                if (!state.userRequestedLive) {
                    state
                } else if (!body.retryable) {
                    terminal(state, body.reason)
                } else {
                    state.copy(errorReason = body.reason, connectedSinceEpochMs = null)
                }

            is ServiceEvent.Fenced.TerminalFailed ->
                terminal(state, body.reason)

            ServiceEvent.Fenced.PermissionRevoked ->
                terminal(state, "Microphone permission removed")

            ServiceEvent.Fenced.AuthFailed ->
                terminal(state, "Authentication failed")

            ServiceEvent.Fenced.AssignmentForbidden ->
                terminal(state, "Not assigned to this session")

            ServiceEvent.Fenced.MalformedContract ->
                terminal(state, "Malformed media contract")

            ServiceEvent.Fenced.AttemptsExhausted ->
                terminal(state, "Reconnect attempts exhausted")

            ServiceEvent.Fenced.ProcessRestored ->
                state.copy(
                    phase = ServicePhase.ProcessRestored,
                    userRequestedLive = false,
                    processRestoredMessage =
                        "Session ended when Android stopped the app",
                    nextRetryAtEpochMs = null,
                    connectedSinceEpochMs = null,
                )

            ServiceEvent.Fenced.StableConnectedReset ->
                state.copy(attemptCount = 0)
        }
    }

    private fun stop(state: ServiceState): ServiceState =
        state.copy(
            phase = ServicePhase.Stopped,
            generation = state.generation + 1L,
            userRequestedLive = false,
            wasExplicitlyStopped = true,
            nextRetryAtEpochMs = null,
            errorReason = null,
            processRestoredMessage = null,
            connectedSinceEpochMs = null,
            inputLevel = 0f,
            outputLevel = 0f,
            stats = null,
            captureSuppressed = false,
        )

    private fun terminal(state: ServiceState, reason: String): ServiceState =
        state.copy(
            phase = ServicePhase.Failed,
            generation = state.generation + 1L,
            userRequestedLive = false,
            errorReason = reason,
            nextRetryAtEpochMs = null,
            connectedSinceEpochMs = null,
            captureSuppressed = false,
        )

    private fun ServicePhase.isLivePhase(): Boolean =
        when (this) {
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

    private fun meterFromEnergy(energy: Double): Float {
        if (energy <= 0.0) return 0f
        // Coarse 0..1 visualization from cumulative energy delta proxy.
        return ((energy % 1.0).toFloat()).coerceIn(0f, 1f)
    }
}
