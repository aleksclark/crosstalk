package com.crosstalk.translator.service

import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.util.Clock
import org.junit.Assert.assertTrue
import org.junit.Test

class MediaTicketValidatorTest {

    private val clock = Clock { 1_000_000L }

    private fun ticket(
        sessionId: String = "sess",
        role: String = "translator",
        expiresAt: Long = 1_000_000L + 30_000L,
        produce: List<String> = listOf("bc-1"),
        listen: List<String> = listOf("feed-1"),
        token: String = "ticket-token",
    ) = MediaTicket(
        token = token,
        sessionId = sessionId,
        role = role,
        expiresAtEpochMs = expiresAt,
        produceChannelIds = produce,
        listenChannelIds = listen,
    )

    @Test
    fun acceptsValidTicket() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(),
                expectedSessionId = "sess",
                requestedFeedIds = listOf("feed-1"),
                requestedBroadcastIds = listOf("bc-1"),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Ok)
    }

    @Test
    fun rejectsSessionMismatch() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(sessionId = "other"),
                expectedSessionId = "sess",
                requestedFeedIds = emptyList(),
                requestedBroadcastIds = emptyList(),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Rejected)
    }

    @Test
    fun rejectsExpired() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(expiresAt = 999_000L),
                expectedSessionId = "sess",
                requestedFeedIds = emptyList(),
                requestedBroadcastIds = emptyList(),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Rejected)
    }

    @Test
    fun rejectsRoleMismatch() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(role = "admin"),
                expectedSessionId = "sess",
                requestedFeedIds = emptyList(),
                requestedBroadcastIds = emptyList(),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Rejected)
    }

    @Test
    fun rejectsChannelWiden() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(produce = listOf("bc-1"), listen = listOf("feed-1")),
                expectedSessionId = "sess",
                requestedFeedIds = listOf("feed-1", "feed-2"),
                requestedBroadcastIds = listOf("bc-1"),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Rejected)
    }

    @Test
    fun allowsClientNarrowing() {
        val r =
            MediaTicketValidator.validate(
                ticket = ticket(produce = listOf("bc-1", "bc-2"), listen = listOf("feed-1", "feed-2")),
                expectedSessionId = "sess",
                requestedFeedIds = listOf("feed-1"),
                requestedBroadcastIds = listOf("bc-1"),
                clock = clock,
            )
        assertTrue(r is MediaTicketValidator.Result.Ok)
    }
}
