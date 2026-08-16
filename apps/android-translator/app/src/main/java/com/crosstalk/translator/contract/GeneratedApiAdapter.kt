package com.crosstalk.translator.contract

import com.crosstalk.translator.generated.api.AuthApi
import com.crosstalk.translator.generated.api.ChannelsApi
import com.crosstalk.translator.generated.api.MixingApi
import com.crosstalk.translator.generated.api.SessionsApi
import com.crosstalk.translator.generated.api.SourcesApi
import com.crosstalk.translator.generated.api.WebRTCApi
import com.crosstalk.translator.generated.infrastructure.ClientException
import com.crosstalk.translator.generated.infrastructure.ServerException
import com.crosstalk.translator.generated.model.ChannelOut
import com.crosstalk.translator.generated.model.LoginRequestBody
import com.crosstalk.translator.generated.model.LogoutRequestBody
import com.crosstalk.translator.generated.model.MixEntryInput
import com.crosstalk.translator.generated.model.MixEntryOut
import com.crosstalk.translator.generated.model.RefreshRequestBody
import com.crosstalk.translator.generated.model.SessionOut
import com.crosstalk.translator.generated.model.SourceOut
import com.crosstalk.translator.generated.model.UpdateMixRequestBody
import com.crosstalk.translator.generated.model.WebRTCTokenRequestBody
import com.crosstalk.translator.generated.model.WebRTCTokenResponseBody
import okhttp3.OkHttpClient
import java.io.IOException
import java.time.OffsetDateTime
import java.util.concurrent.atomic.AtomicReference

/**
 * Sole handwritten bridge to generated OpenAPI clients.
 * Maps generated DTOs to domain models and never leaks them outward.
 */
class GeneratedApiAdapter(
    baseUrl: String,
    client: OkHttpClient,
) : CrossTalkApi {
    private val normalizedBaseUrl = baseUrl.trimEnd('/')
    private val authApi = AuthApi(normalizedBaseUrl, client)
    private val sessionsApi = SessionsApi(normalizedBaseUrl, client)
    private val channelsApi = ChannelsApi(normalizedBaseUrl, client)
    private val sourcesApi = SourcesApi(normalizedBaseUrl, client)
    private val mixingApi = MixingApi(normalizedBaseUrl, client)
    private val webRtcApi = WebRTCApi(normalizedBaseUrl, client)

    private val accessTokenRef = AtomicReference<String?>(null)

    fun setAccessToken(token: String?) {
        accessTokenRef.set(token)
    }

    fun getAccessToken(): String? = accessTokenRef.get()

    private fun bearerHeader(): String? =
        accessTokenRef.get()?.takeIf { it.isNotBlank() }?.let { "Bearer $it" }

    override suspend fun login(username: String, password: String): AuthTokens =
        mapErrors {
            val body = authApi.login(
                LoginRequestBody(
                    username = username,
                    password = password,
                ),
            )
            AuthTokens(
                accessToken = body.accessToken,
                refreshToken = body.refreshToken,
            )
        }

    override suspend fun refresh(refreshToken: String): AuthTokens =
        mapErrors {
            val body = authApi.refresh(RefreshRequestBody(refreshToken = refreshToken))
            AuthTokens(
                accessToken = body.accessToken,
                refreshToken = body.refreshToken,
            )
        }

    override suspend fun logout(refreshToken: String) {
        mapErrors {
            authApi.logout(LogoutRequestBody(refreshToken = refreshToken))
            Unit
        }
    }

    override suspend fun listSessions(): List<SessionSummary> =
        mapErrors {
            val body = sessionsApi.listSessions(authorization = bearerHeader())
            body.data.orEmpty().map { it.toSummary() }
        }

    override suspend fun getSession(sessionId: String): SessionSummary =
        mapErrors {
            sessionsApi.getSession(id = sessionId, authorization = bearerHeader()).toSummary()
        }

    override suspend fun listChannels(sessionId: String): List<ChannelInfo> =
        mapErrors {
            val body = channelsApi.listChannels(id = sessionId, authorization = bearerHeader())
            body.data.orEmpty().map { it.toInfo() }
        }

    override suspend fun getBroadcastLink(sessionId: String): BroadcastLink =
        mapErrors {
            val body = sessionsApi.getBroadcastUrl(id = sessionId, authorization = bearerHeader())
            BroadcastLink(
                token = body.broadcastToken,
                url = body.url,
            )
        }

    override suspend fun listSources(sessionId: String): List<SourceInfo> =
        mapErrors {
            sourcesApi.listSources(id = sessionId, authorization = bearerHeader())
                .data.orEmpty()
                .map { it.toInfo() }
        }

    override suspend fun getMix(sessionId: String, channelId: String): List<MixEntry> =
        mapErrors {
            mixingApi.getMix(
                id = sessionId,
                chId = channelId,
                authorization = bearerHeader(),
            ).data.orEmpty().map { it.toEntry() }
        }

    override suspend fun updateMix(
        sessionId: String,
        channelId: String,
        entries: List<MixEntry>,
    ): List<MixEntry> =
        mapErrors {
            val body = UpdateMixRequestBody(
                propertyEntries = entries.map { entry ->
                    MixEntryInput(
                        sourceId = entry.sourceId,
                        muted = entry.muted,
                        level = entry.level.coerceIn(0.0, 2.0),
                    )
                },
            )
            mixingApi.updateMix(
                id = sessionId,
                chId = channelId,
                updateMixRequestBody = body,
                authorization = bearerHeader(),
            ).data.orEmpty().map { it.toEntry() }
        }

    override suspend fun mintMediaTicket(sessionId: String): MediaTicket =
        mapErrors {
            // Translator app hard-codes role. Omit produce/listen so the server applies
            // translator defaults (broadcast produce, feed listen). Never elevate.
            val body = webRtcApi.getWebrtcToken(
                webRTCTokenRequestBody = WebRTCTokenRequestBody(
                    sessionId = sessionId,
                    role = WebRTCTokenRequestBody.Role.TRANSLATOR,
                ),
                authorization = bearerHeader(),
            )
            body.toMediaTicket()
        }

    private fun SessionOut.toSummary(): SessionSummary =
        SessionSummary(
            id = id,
            name = name,
            description = description.takeIf { it.isNotBlank() },
            status = null,
        )

    private fun ChannelOut.toInfo(): ChannelInfo =
        ChannelInfo(
            id = id,
            name = name,
            type = type,
            sessionId = sessionId,
        )

    private fun SourceOut.toInfo(): SourceInfo =
        SourceInfo(
            id = id,
            name = name,
            origin = origin,
            connected = connected,
        )

    private fun MixEntryOut.toEntry(): MixEntry =
        MixEntry(
            id = id,
            channelId = channelId,
            sourceId = sourceId,
            muted = muted,
            level = level,
        )

    private fun WebRTCTokenResponseBody.toMediaTicket(): MediaTicket =
        MediaTicket(
            token = token,
            sessionId = sessionId,
            role = role,
            expiresAtEpochMs = expiresAt.toEpochMillis(),
            produceChannelIds = produceChannelIds.orEmpty(),
            listenChannelIds = listenChannelIds.orEmpty(),
        )

    private fun OffsetDateTime.toEpochMillis(): Long =
        toInstant().toEpochMilli()

    private suspend fun <T> mapErrors(block: suspend () -> T): T {
        try {
            return block()
        } catch (e: ClientException) {
            throw when (e.statusCode) {
                401 -> ApiException.Unauthorized(
                    message = e.message ?: "Unauthorized",
                    cause = e,
                )
                403 -> ApiException.Forbidden(
                    message = e.message ?: "Forbidden",
                    cause = e,
                )
                else -> ApiException.Client(
                    message = e.message ?: "Client error",
                    statusCode = e.statusCode,
                    cause = e,
                )
            }
        } catch (e: ServerException) {
            throw ApiException.Server(
                message = e.message ?: "Server error",
                statusCode = e.statusCode,
                cause = e,
            )
        } catch (e: IOException) {
            throw ApiException.Network(
                message = e.message ?: "Network error",
                cause = e,
            )
        } catch (e: ApiException) {
            throw e
        } catch (e: Exception) {
            throw ApiException.Unexpected(
                message = e.message ?: "Unexpected API failure",
                cause = e,
            )
        }
    }
}
