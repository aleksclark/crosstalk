package com.crosstalk.translator.service

import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.util.Clock

/**
 * Fail-closed media-ticket validation before signaling.
 * Client selectors may narrow channel scope, never widen.
 */
object MediaTicketValidator {

    sealed class Result {
        data object Ok : Result()
        data class Rejected(val reason: String) : Result()
    }

    fun validate(
        ticket: MediaTicket,
        expectedSessionId: String,
        requestedFeedIds: List<String>,
        requestedBroadcastIds: List<String>,
        clock: Clock,
        expectedRole: String = "translator",
    ): Result {
        if (ticket.token.isBlank()) {
            return Result.Rejected("Empty media ticket")
        }
        if (ticket.sessionId != expectedSessionId) {
            return Result.Rejected("Ticket session mismatch")
        }
        if (!ticket.role.equals(expectedRole, ignoreCase = true)) {
            return Result.Rejected("Ticket role mismatch")
        }
        if (ticket.expiresAtEpochMs <= clock.nowEpochMs()) {
            return Result.Rejected("Ticket expired")
        }
        // Produce = broadcast for translator; listen = feed.
        if (requestedBroadcastIds.isNotEmpty()) {
            val produce = ticket.produceChannelIds.toSet()
            if (produce.isNotEmpty() && !produce.containsAll(requestedBroadcastIds)) {
                return Result.Rejected("Ticket produce channels do not cover selection")
            }
        }
        if (requestedFeedIds.isNotEmpty()) {
            val listen = ticket.listenChannelIds.toSet()
            if (listen.isNotEmpty() && !listen.containsAll(requestedFeedIds)) {
                return Result.Rejected("Ticket listen channels do not cover selection")
            }
        }
        return Result.Ok
    }
}
