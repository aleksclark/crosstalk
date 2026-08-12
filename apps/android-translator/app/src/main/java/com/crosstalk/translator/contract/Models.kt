package com.crosstalk.translator.contract

/**
 * Domain-facing models consumed by auth/UI/service layers.
 * Generated OpenAPI DTOs must not leak outside [contract].
 */

data class AuthTokens(
    val accessToken: String,
    val refreshToken: String,
    val tokenType: String = "Bearer",
    val expiresInSeconds: Long? = null,
)

data class SessionSummary(
    val id: String,
    val name: String,
    val description: String? = null,
    val status: String? = null,
)

data class ChannelInfo(
    val id: String,
    val name: String,
    val type: String,
    val sessionId: String,
)

data class MediaTicket(
    val token: String,
    val sessionId: String,
    val role: String,
    val expiresAtEpochMs: Long,
    val produceChannelIds: List<String> = emptyList(),
    val listenChannelIds: List<String> = emptyList(),
)

data class LastSession(
    val sessionId: String,
    val sessionName: String,
    val feedChannelName: String? = null,
    val broadcastChannelName: String? = null,
    val wasExplicitlyStopped: Boolean = false,
)
