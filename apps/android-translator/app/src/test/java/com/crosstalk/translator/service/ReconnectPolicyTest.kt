package com.crosstalk.translator.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReconnectPolicyTest {

    @Test
    fun fullJitterIsWithinCapAndBase() {
        val policy = ReconnectPolicy(random01 = { 1.0 })
        // Max random → delay equals ceiling, capped at 30s.
        assertEquals(1_000L, policy.fullJitterDelayMs(0))
        assertEquals(2_000L, policy.fullJitterDelayMs(1))
        assertEquals(4_000L, policy.fullJitterDelayMs(2))
        assertEquals(30_000L, policy.fullJitterDelayMs(10))

        val zero = ReconnectPolicy(random01 = { 0.0 })
        assertEquals(0L, zero.fullJitterDelayMs(0))
        assertEquals(0L, zero.fullJitterDelayMs(5))
    }

    @Test
    fun schedulesRetryWithIncrementedAttempt() {
        val policy = ReconnectPolicy(random01 = { 0.5 })
        val budget = policy.onUserLiveStarted(nowEpochMs = 1_000L)
        val decision =
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Ice,
                nowEpochMs = 1_500L,
                networkValidated = true,
            )
        assertTrue(decision is ReconnectPolicy.Decision.ScheduleRetry)
        val schedule = decision as ReconnectPolicy.Decision.ScheduleRetry
        assertEquals(1, schedule.next.attemptCount)
        assertEquals(500L, schedule.delayMs) // 0.5 * 1000
    }

    @Test
    fun maxAttemptsExhausted() {
        val policy = ReconnectPolicy(random01 = { 0.0 })
        var budget = policy.onUserLiveStarted(0L)
        budget = budget.copy(attemptCount = ReconnectPolicy.MAX_ATTEMPTS)
        val decision =
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Transport,
                nowEpochMs = 1_000L,
                networkValidated = true,
            )
        assertTrue(decision is ReconnectPolicy.Decision.GiveUp)
        assertTrue((decision as ReconnectPolicy.Decision.GiveUp).reason.contains("attempts"))
    }

    @Test
    fun maxWindowExhausted() {
        val policy = ReconnectPolicy(random01 = { 0.0 })
        val start = 0L
        val budget =
            ReconnectPolicy.Budget(
                attemptCount = 1,
                windowStartEpochMs = start,
            )
        val decision =
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Transport,
                nowEpochMs = start + ReconnectPolicy.MAX_WINDOW_MS + 1,
                networkValidated = true,
            )
        assertTrue(decision is ReconnectPolicy.Decision.GiveUp)
        assertTrue((decision as ReconnectPolicy.Decision.GiveUp).reason.contains("window"))
    }

    @Test
    fun networkUnavailablePausesBudget() {
        val policy = ReconnectPolicy(random01 = { 0.5 })
        val budget = policy.onUserLiveStarted(0L)
        val decision =
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Transport,
                nowEpochMs = 100L,
                networkValidated = false,
            )
        assertEquals(ReconnectPolicy.Decision.WaitForNetwork, decision)

        val networkKind =
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Network,
                nowEpochMs = 100L,
                networkValidated = true,
            )
        assertEquals(ReconnectPolicy.Decision.WaitForNetwork, networkKind)
    }

    @Test
    fun nonRetryableFailures() {
        val policy = ReconnectPolicy()
        val budget = policy.onUserLiveStarted(0L)
        for (
        kind in listOf(
            ReconnectPolicy.FailureKind.Auth,
            ReconnectPolicy.FailureKind.Forbidden,
            ReconnectPolicy.FailureKind.MicPermission,
            ReconnectPolicy.FailureKind.MalformedContract,
            ReconnectPolicy.FailureKind.UnsupportedCodec,
            ReconnectPolicy.FailureKind.PermanentFocusLoss,
        )
        ) {
            assertFalse(policy.isRetryable(kind))
            val d =
                policy.decide(
                    budget = budget,
                    kind = kind,
                    nowEpochMs = 0L,
                    networkValidated = true,
                )
            assertTrue("$kind should give up", d is ReconnectPolicy.Decision.GiveUp)
        }
        assertEquals(
            ReconnectPolicy.Decision.DoNotRetry,
            policy.decide(
                budget = budget,
                kind = ReconnectPolicy.FailureKind.Stop,
                nowEpochMs = 0L,
                networkValidated = true,
            ),
        )
    }

    @Test
    fun stableConnectedResetsAttemptsAfter30s() {
        val policy = ReconnectPolicy()
        var budget =
            ReconnectPolicy.Budget(
                attemptCount = 4,
                windowStartEpochMs = 0L,
                connectedSinceEpochMs = 10_000L,
            )
        // Not yet stable.
        budget = policy.onStableConnectedCheck(budget, nowEpochMs = 10_000L + 29_000L)
        assertEquals(4, budget.attemptCount)

        budget = policy.onStableConnectedCheck(budget, nowEpochMs = 10_000L + 30_000L)
        assertEquals(0, budget.attemptCount)
    }

    @Test
    fun onNetworkLostClearsConnectedSince() {
        val policy = ReconnectPolicy()
        val budget =
            policy.onConnected(
                policy.onUserLiveStarted(0L).copy(attemptCount = 2),
                nowEpochMs = 5_000L,
            )
        val lost = policy.onNetworkLost(budget)
        assertTrue(lost.pausedForNetwork)
        assertEquals(null, lost.connectedSinceEpochMs)
        assertEquals(2, lost.attemptCount)
    }
}
