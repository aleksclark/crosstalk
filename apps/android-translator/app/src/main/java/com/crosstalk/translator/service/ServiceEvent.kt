package com.crosstalk.translator.service

import com.crosstalk.translator.rtc.RtcStats

/**
 * Internal reducer inputs. Commands arrive from UI; other events from service orchestration.
 * Every event carries [generation] except raw [Command] (reducer assigns/fences).
 */
sealed class ServiceEvent {
    data class Command(val command: ServiceCommand) : ServiceEvent()

    data class GenerationFenced(
        val generation: Long,
        val body: Fenced,
    ) : ServiceEvent()

    sealed class Fenced {
        data object Preparing : Fenced()
        data object Minting : Fenced()
        data object Signaling : Fenced()
        data object IceChecking : Fenced()
        data object Connected : Fenced()
        data class Stats(val stats: RtcStats) : Fenced()
        data class Levels(val input: Float, val output: Float) : Fenced()
        data class MutedApplied(val muted: Boolean) : Fenced()
        data object TransientFocusLost : Fenced()
        data object FocusRegained : Fenced()
        data object PermanentFocusLost : Fenced()
        data object NetworkLost : Fenced()
        data object NetworkValidated : Fenced()
        data class RetryScheduled(
            val attemptCount: Int,
            val nextRetryAtEpochMs: Long,
        ) : Fenced()
        data class TransportFailed(
            val reason: String,
            val retryable: Boolean,
        ) : Fenced()
        data class TerminalFailed(val reason: String) : Fenced()
        data object PermissionRevoked : Fenced()
        data object AuthFailed : Fenced()
        data object AssignmentForbidden : Fenced()
        data object MalformedContract : Fenced()
        data object AttemptsExhausted : Fenced()
        data object ProcessRestored : Fenced()
        data object StableConnectedReset : Fenced()
    }
}
