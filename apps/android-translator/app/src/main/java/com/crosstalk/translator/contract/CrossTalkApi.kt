package com.crosstalk.translator.contract

/**
 * Narrow REST surface used by auth and the foreground service.
 * Implementations live in this package (generated adapter) only.
 */
interface CrossTalkApi {
    suspend fun login(username: String, password: String): AuthTokens
    suspend fun refresh(refreshToken: String): AuthTokens
    suspend fun logout(refreshToken: String)
    suspend fun listSessions(): List<SessionSummary>
    suspend fun getSession(sessionId: String): SessionSummary
    suspend fun listChannels(sessionId: String): List<ChannelInfo>
    /** Always mints a translator-scoped one-time media ticket (server derives capabilities). */
    suspend fun mintMediaTicket(sessionId: String): MediaTicket
}
