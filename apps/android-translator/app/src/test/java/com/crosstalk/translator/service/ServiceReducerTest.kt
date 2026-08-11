package com.crosstalk.translator.service

import com.crosstalk.translator.rtc.RtcStats
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ServiceReducerTest {

    private val join =
        ServiceCommand.Join(
            sessionId = "sess-1",
            sessionName = "Sunday Spanish",
            feedName = "Floor Feed",
            broadcastName = "English Broadcast",
            feedIds = listOf("feed-1"),
            broadcastIds = listOf("bc-1"),
        )

    @Test
    fun joinTransitionsToPreparingAndBumpsGeneration() {
        val next = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        assertEquals(ServicePhase.Preparing, next.phase)
        assertEquals(1L, next.generation)
        assertEquals("Sunday Spanish", next.sessionName)
        assertEquals("Floor Feed", next.feedName)
        assertEquals("English Broadcast", next.broadcastName)
        assertTrue(next.userRequestedLive)
        assertFalse(next.wasExplicitlyStopped)
    }

    @Test
    fun stopAlwaysWinsAndFencesGeneration() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )
        assertEquals(ServicePhase.Connected, state.phase)

        state = ServiceReducer.reduce(state, ServiceEvent.Command(ServiceCommand.Stop))
        assertEquals(ServicePhase.Stopped, state.phase)
        assertTrue(state.wasExplicitlyStopped)
        assertFalse(state.userRequestedLive)
        assertEquals(gen + 1L, state.generation)

        // Stale connected callback must not resurrect live media.
        val stale =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )
        assertEquals(ServicePhase.Stopped, stale.phase)
        assertFalse(stale.userRequestedLive)
    }

    @Test
    fun stopWinsOverPendingRetry() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(
                    gen,
                    ServiceEvent.Fenced.RetryScheduled(attemptCount = 2, nextRetryAtEpochMs = 9_000),
                ),
            )
        assertEquals(ServicePhase.ReconnectScheduled, state.phase)
        assertEquals(2, state.attemptCount)

        state = ServiceReducer.reduce(state, ServiceEvent.Command(ServiceCommand.Stop))
        val afterStopGen = state.generation

        val afterStaleRetry =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(
                    gen,
                    ServiceEvent.Fenced.RetryScheduled(3, 12_000),
                ),
            )
        assertEquals(ServicePhase.Stopped, afterStaleRetry.phase)
        assertNull(afterStaleRetry.nextRetryAtEpochMs)
        assertEquals(afterStopGen, afterStaleRetry.generation)
    }

    @Test
    fun generationFencingDropsStaleEvents() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen1 = state.generation
        // Simulate replace join
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.Command(join.copy(sessionId = "sess-2", sessionName = "Other")),
            )
        val gen2 = state.generation
        assertNotEquals(gen1, gen2)

        val ignored =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen1, ServiceEvent.Fenced.Connected),
            )
        assertEquals(ServicePhase.Preparing, ignored.phase)
        assertEquals("Other", ignored.sessionName)
    }

    @Test
    fun mutePreservesConnectedAndFocusTransientDoesNotClearManualMute() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )

        state =
            ServiceReducer.reduce(state, ServiceEvent.Command(ServiceCommand.SetMuted(true)))
        assertEquals(ServicePhase.Muted, state.phase)
        assertTrue(state.micMuted)
        assertTrue(state.captureSuppressed)

        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.TransientFocusLost),
            )
        assertTrue(state.captureSuppressed)
        assertTrue(state.micMuted)

        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.FocusRegained),
            )
        // Manual mute preserved.
        assertTrue(state.micMuted)
        assertTrue(state.captureSuppressed)
        assertEquals(ServicePhase.Muted, state.phase)
    }

    @Test
    fun transientFocusLossSuppressesCaptureWithoutManualMute() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )
        assertFalse(state.micMuted)

        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.TransientFocusLost),
            )
        assertTrue(state.captureSuppressed)
        assertFalse(state.micMuted)

        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.FocusRegained),
            )
        assertFalse(state.captureSuppressed)
        assertFalse(state.micMuted)
    }

    @Test
    fun permissionRevocationIsTerminal() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.PermissionRevoked),
            )
        assertEquals(ServicePhase.Failed, state.phase)
        assertEquals("Microphone permission removed", state.errorReason)
        assertFalse(state.userRequestedLive)
        // Generation bumped so stale reconnect cannot continue.
        assertEquals(gen + 1L, state.generation)
    }

    @Test
    fun authAndForbiddenAreTerminal() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        val auth =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.AuthFailed),
            )
        assertEquals(ServicePhase.Failed, auth.phase)
        assertEquals("Authentication failed", auth.errorReason)

        state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val g2 = state.generation
        val forbidden =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(g2, ServiceEvent.Fenced.AssignmentForbidden),
            )
        assertEquals("Not assigned to this session", forbidden.errorReason)
    }

    @Test
    fun networkLostAndValidated() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.NetworkLost),
            )
        assertEquals(ServicePhase.WaitingForNetwork, state.phase)

        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.NetworkValidated),
            )
        assertEquals(ServicePhase.ReconnectScheduled, state.phase)
    }

    @Test
    fun processRestoredMessage() {
        var state =
            ServiceState(
                phase = ServicePhase.Idle,
                sessionId = "s",
                sessionName = "Sunday Spanish",
            )
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(0L, ServiceEvent.Fenced.ProcessRestored),
            )
        assertEquals(ServicePhase.ProcessRestored, state.phase)
        assertEquals(
            "Session ended when Android stopped the app",
            state.processRestoredMessage,
        )
        assertFalse(state.userRequestedLive)
    }

    @Test
    fun permanentFocusLossTerminal() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.PermanentFocusLost),
            )
        assertEquals(ServicePhase.Failed, state.phase)
        assertEquals("Audio focus permanently lost", state.errorReason)
    }

    @Test
    fun statsUpdateLevels() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        val stats =
            RtcStats(
                audioLevel = 0.4,
                totalAudioEnergy = 1.2,
                bytesReceived = 10,
                bytesSent = 5,
            )
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Stats(stats)),
            )
        assertEquals(stats, state.stats)
        assertEquals(0.4f, state.inputLevel, 0.001f)
    }

    @Test
    fun connectedPhaseHonorsManualMute() {
        var state = ServiceReducer.reduce(ServiceState.Idle, ServiceEvent.Command(join))
        val gen = state.generation
        state =
            ServiceReducer.reduce(state, ServiceEvent.Command(ServiceCommand.SetMuted(true)))
        state =
            ServiceReducer.reduce(
                state,
                ServiceEvent.GenerationFenced(gen, ServiceEvent.Fenced.Connected),
            )
        assertEquals(ServicePhase.Muted, state.phase)
    }
}
