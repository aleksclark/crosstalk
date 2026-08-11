package com.crosstalk.translator.app

import android.content.Context
import com.crosstalk.translator.BuildConfig
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.CredentialVault
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.GeneratedApiAdapter
import com.crosstalk.translator.network.ApiClientFactory
import com.crosstalk.translator.network.TlsPolicy
import com.crosstalk.translator.rtc.LibWebRtcEngine
import com.crosstalk.translator.rtc.RtcEngine
import com.crosstalk.translator.service.AudioServiceGateway
import com.crosstalk.translator.service.BoundAudioServiceGateway
import com.crosstalk.translator.service.LastSessionStore
import com.crosstalk.translator.util.SystemClock
import okhttp3.OkHttpClient

/**
 * Manual constructor-injection graph for the single-module app.
 */
class AppContainer(
    context: Context,
) {
    private val appContext = context.applicationContext

    val clock = SystemClock()

    val apiBaseUrl: String = BuildConfig.API_BASE_URL
    val allowCleartext: Boolean = BuildConfig.ALLOW_CLEARTEXT
    val deploymentIdentity: String = apiBaseUrl

    init {
        TlsPolicy.assertSafeBaseUrl(apiBaseUrl, allowCleartext = allowCleartext)
    }

    val okHttpClient: OkHttpClient by lazy {
        ApiClientFactory.create(
            allowCleartext = allowCleartext,
        )
    }

    private val generatedApi: GeneratedApiAdapter by lazy {
        GeneratedApiAdapter(
            baseUrl = apiBaseUrl,
            client = okHttpClient,
        )
    }

    val api: CrossTalkApi
        get() = generatedApi

    val credentialVault: CredentialVault by lazy {
        CredentialVault.create(appContext)
    }

    val authRepository: AuthRepository by lazy {
        AuthRepository(
            api = generatedApi,
            vault = credentialVault,
            accessTokenSink = { token -> generatedApi.setAccessToken(token) },
        )
    }

    val lastSessionStore: LastSessionStore by lazy {
        LastSessionStore(appContext)
    }

    /**
     * Production factory: each connect/reconnect attempt may construct a fresh engine.
     * Tests / instrumentation may replace this before service start.
     */
    @Volatile
    var rtcEngineFactory: () -> RtcEngine = {
        LibWebRtcEngine(
            appContext = appContext,
            httpClient = okHttpClient,
        )
    }

    val audioServiceGateway: AudioServiceGateway by lazy {
        BoundAudioServiceGateway(appContext)
    }

    fun applicationContext(): Context = appContext

    /**
     * Derive WSS/WS signaling base from REST API base URL.
     * https://host → wss://host ; http://host → ws://host
     */
    fun wsBaseUrl(): String = httpToWsBase(apiBaseUrl)

    companion object {
        fun httpToWsBase(apiBaseUrl: String): String {
            val trimmed = apiBaseUrl.trim().trimEnd('/')
            return when {
                trimmed.startsWith("https://", ignoreCase = true) ->
                    "wss://" + trimmed.removePrefix("https://").removePrefix("HTTPS://")
                trimmed.startsWith("http://", ignoreCase = true) ->
                    "ws://" + trimmed.removePrefix("http://").removePrefix("HTTP://")
                trimmed.startsWith("wss://", ignoreCase = true) ||
                    trimmed.startsWith("ws://", ignoreCase = true) -> trimmed
                else -> "wss://$trimmed"
            }
        }
    }
}
