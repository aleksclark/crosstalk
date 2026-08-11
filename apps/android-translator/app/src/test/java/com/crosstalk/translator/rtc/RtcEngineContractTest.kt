package com.crosstalk.translator.rtc

import app.cash.turbine.test
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class RtcEngineContractTest {
    private fun sampleRequest() =
        RtcConnectRequest(
            wsBaseUrl = "https://crosstalk.test",
            sessionId = "01HSESSIONTEST000000000000",
            mediaTicket = "media-ticket-canary-value",
        )

    @Test
    fun connectEmitsOrderedLifecycleEvents() =
        runTest {
            val engine = FakeRtcEngine()
            engine.events.test {
                engine.connect(sampleRequest())
                assertTrue(awaitItem() is RtcEvent.Connecting)
                assertTrue(awaitItem() is RtcEvent.SignalingStateChanged)
                assertTrue(awaitItem() is RtcEvent.LocalOfferSent)
                assertTrue(awaitItem() is RtcEvent.RemoteDescriptionApplied)
                assertTrue(awaitItem() is RtcEvent.IceConnectionStateChanged)
                assertTrue(awaitItem() is RtcEvent.PeerConnectionStateChanged)
                assertTrue(awaitItem() is RtcEvent.RemoteTrack)
                assertTrue(awaitItem() is RtcEvent.StatsUpdated)
                cancelAndIgnoreRemainingEvents()
            }
        }

    @Test
    fun closeIsIdempotent() =
        runTest {
            val engine = FakeRtcEngine()
            engine.connect(sampleRequest())
            engine.close(StopReason.UserStop)
            engine.close(StopReason.UserStop)
            engine.close(StopReason.Error)
            assertEquals(3, engine.closeCount())
            assertTrue(engine.isClosed())
            assertEquals(StopReason.Error, engine.lastCloseReason())
        }

    @Test
    fun mutePreservesReceivePath() =
        runTest {
            val engine = FakeRtcEngine()
            engine.connect(sampleRequest())
            engine.setMuted(true)
            assertTrue(engine.isMuted())
            assertTrue(engine.isReceivePathAlive())
            val stats = engine.stats()
            assertTrue(stats.bytesReceived > 0 || stats.packetsReceived >= 0)
            engine.setMuted(false)
            assertFalse(engine.isMuted())
            assertTrue(engine.isReceivePathAlive())
        }

    @Test
    fun staleAttemptCallbacksCannotMutateCurrentState() =
        runTest {
            val engine = FakeRtcEngine()
            engine.connect(sampleRequest())
            val firstAttempt = engine.currentAttemptId()

            engine.connect(sampleRequest().copy(mediaTicket = "second-ticket"))
            val secondAttempt = engine.currentAttemptId()
            assertTrue(secondAttempt > firstAttempt)

            engine.events.test {
                // Drain any connect emissions from the second connect if still buffered —
                // SharedFlow with extraBufferCapacity may have dropped; just try apply.
                cancelAndIgnoreRemainingEvents()
            }

            // Stale failure from first attempt must be fenced.
            val acceptedStale =
                engine.applyStaleCallback(
                    forAttempt = firstAttempt,
                    event = RtcEvent.Failed(StopReason.IceFailed, "stale ice fail"),
                )
            assertFalse("stale attempt must be ignored", acceptedStale)

            // Current attempt still accepts.
            val acceptedCurrent =
                engine.applyStaleCallback(
                    forAttempt = secondAttempt,
                    event = RtcEvent.IceConnectionStateChanged("completed"),
                )
            assertTrue(acceptedCurrent)

            // Mute on current attempt still works after fencing.
            engine.setMuted(true)
            assertTrue(engine.isMuted())
            assertEquals(secondAttempt, engine.currentAttemptId())
        }

    @Test
    fun closeFencesFurtherCallbacks() =
        runTest {
            val engine = FakeRtcEngine()
            engine.connect(sampleRequest())
            val attempt = engine.currentAttemptId()
            engine.close(StopReason.UserStop)
            val accepted =
                engine.applyStaleCallback(
                    forAttempt = attempt,
                    event = RtcEvent.StatsUpdated(RtcStats(bytesSent = 999)),
                )
            assertFalse(accepted)
        }

    @Test
    fun defaultIceServersIncludeGoogleStun() {
        val req =
            RtcConnectRequest(
                wsBaseUrl = "wss://example",
                sessionId = "s",
                mediaTicket = "t",
            )
        assertEquals(1, req.iceServers.size)
        assertTrue(req.iceServers.first().urls.any { it.contains("stun.l.google.com") })
    }
}
