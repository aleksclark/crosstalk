package com.crosstalk.translator.service

/**
 * Pure reconnect policy: full-jitter backoff, attempt/time caps, network pause,
 * and non-retryable failure classes.
 *
 * Full jitter: `delay = random * min(cap, base * 2^attempt)` with base 1s, cap 30s.
 * Reset after 30s continuously connected. Max 10 attempts or 5 minutes window.
 */
class ReconnectPolicy(
    private val random01: () -> Double = { Math.random() },
) {
    data class Budget(
        val attemptCount: Int = 0,
        val windowStartEpochMs: Long? = null,
        val connectedSinceEpochMs: Long? = null,
        val pausedForNetwork: Boolean = false,
    )

    enum class FailureKind {
        Transport,
        Ice,
        Signaling,
        Network,
        Auth,
        Forbidden,
        MicPermission,
        MalformedContract,
        UnsupportedCodec,
        PermanentFocusLoss,
        Stop,
        Unknown,
    }

    sealed class Decision {
        data class ScheduleRetry(
            val delayMs: Long,
            val next: Budget,
        ) : Decision()

        data object WaitForNetwork : Decision()

        data class GiveUp(val reason: String) : Decision()

        data object DoNotRetry : Decision()
    }

    fun onUserLiveStarted(nowEpochMs: Long): Budget =
        Budget(attemptCount = 0, windowStartEpochMs = nowEpochMs)

    fun onConnected(budget: Budget, nowEpochMs: Long): Budget =
        budget.copy(
            connectedSinceEpochMs = nowEpochMs,
            pausedForNetwork = false,
        )

    /**
     * After [STABLE_CONNECTED_MS] of continuous connection, reset attempt budget.
     */
    fun onStableConnectedCheck(budget: Budget, nowEpochMs: Long): Budget {
        val since = budget.connectedSinceEpochMs ?: return budget
        if (nowEpochMs - since < STABLE_CONNECTED_MS) return budget
        return Budget(
            attemptCount = 0,
            windowStartEpochMs = nowEpochMs,
            connectedSinceEpochMs = since,
            pausedForNetwork = false,
        )
    }

    fun onNetworkLost(budget: Budget): Budget =
        budget.copy(pausedForNetwork = true, connectedSinceEpochMs = null)

    fun onNetworkValidated(budget: Budget): Budget =
        budget.copy(pausedForNetwork = false)

    fun isRetryable(kind: FailureKind): Boolean =
        when (kind) {
            FailureKind.Transport,
            FailureKind.Ice,
            FailureKind.Signaling,
            FailureKind.Network,
            FailureKind.Unknown,
            -> true
            FailureKind.Auth,
            FailureKind.Forbidden,
            FailureKind.MicPermission,
            FailureKind.MalformedContract,
            FailureKind.UnsupportedCodec,
            FailureKind.PermanentFocusLoss,
            FailureKind.Stop,
            -> false
        }

    fun decide(
        budget: Budget,
        kind: FailureKind,
        nowEpochMs: Long,
        networkValidated: Boolean,
    ): Decision {
        if (kind == FailureKind.Stop) return Decision.DoNotRetry
        if (!isRetryable(kind)) {
            return Decision.GiveUp(reason = nonRetryableReason(kind))
        }
        if (!networkValidated || kind == FailureKind.Network) {
            return Decision.WaitForNetwork
        }

        val windowStart = budget.windowStartEpochMs ?: nowEpochMs
        val nextAttempt = budget.attemptCount + 1
        if (nextAttempt > MAX_ATTEMPTS) {
            return Decision.GiveUp(reason = "Reconnect attempts exhausted ($MAX_ATTEMPTS)")
        }
        if (nowEpochMs - windowStart > MAX_WINDOW_MS) {
            return Decision.GiveUp(reason = "Reconnect window exhausted (5 minutes)")
        }

        val delay = fullJitterDelayMs(attemptIndexZeroBased = budget.attemptCount)
        val next = Budget(
            attemptCount = nextAttempt,
            windowStartEpochMs = windowStart,
            connectedSinceEpochMs = null,
            pausedForNetwork = false,
        )
        return Decision.ScheduleRetry(delayMs = delay, next = next)
    }

    /**
     * Full jitter for the upcoming retry after [attemptIndexZeroBased] prior failures
     * in the current window (0 => first retry).
     */
    fun fullJitterDelayMs(attemptIndexZeroBased: Int): Long {
        val exp = attemptIndexZeroBased.coerceAtLeast(0).coerceAtMost(16)
        val ceiling = (BASE_DELAY_MS.toDouble() * (1L shl exp).toDouble())
            .toLong()
            .coerceAtMost(MAX_DELAY_MS)
            .coerceAtLeast(BASE_DELAY_MS)
        val r = random01().coerceIn(0.0, 1.0)
        return (r * ceiling).toLong().coerceIn(0L, MAX_DELAY_MS)
    }

    private fun nonRetryableReason(kind: FailureKind): String =
        when (kind) {
            FailureKind.Auth -> "Authentication failed"
            FailureKind.Forbidden -> "Not assigned to this session"
            FailureKind.MicPermission -> "Microphone permission removed"
            FailureKind.MalformedContract -> "Malformed media contract"
            FailureKind.UnsupportedCodec -> "Unsupported audio codec"
            FailureKind.PermanentFocusLoss -> "Audio focus permanently lost"
            FailureKind.Stop -> "Stopped"
            else -> "Non-retryable failure"
        }

    companion object {
        const val BASE_DELAY_MS: Long = 1_000L
        const val MAX_DELAY_MS: Long = 30_000L
        const val MAX_ATTEMPTS: Int = 10
        const val MAX_WINDOW_MS: Long = 5 * 60_000L
        const val STABLE_CONNECTED_MS: Long = 30_000L
        const val ICE_DISCONNECT_GRACE_MS: Long = 5_000L
    }
}
