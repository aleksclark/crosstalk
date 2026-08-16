package com.crosstalk.translator.app

import android.content.Context
import com.crosstalk.translator.BuildConfig
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.CredentialVault
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.GeneratedApiAdapter
import com.crosstalk.translator.network.ApiClientFactory
import com.crosstalk.translator.network.ServerEndpoint
import com.crosstalk.translator.network.ServerUrlStore
import com.crosstalk.translator.network.TlsPolicy
import com.crosstalk.translator.rtc.LibWebRtcEngine
import com.crosstalk.translator.rtc.RtcEngine
import com.crosstalk.translator.service.AudioServiceGateway
import com.crosstalk.translator.service.BoundAudioServiceGateway
import com.crosstalk.translator.service.LastSessionStore
import com.crosstalk.translator.util.SystemClock
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import java.util.concurrent.atomic.AtomicReference

/**
 * Manual constructor-injection graph for the single-module app.
 */
class AppContainer(
    context: Context,
    private val credentialVaultOverride: CredentialVault? = null,
    serverUrlStoreOverride: ServerUrlStore? = null,
    private val audioServiceGatewayOverride: AudioServiceGateway? = null,
) {
    private val appContext = context.applicationContext

    val clock = SystemClock()

    val allowCleartext: Boolean = BuildConfig.ALLOW_CLEARTEXT

    val okHttpClient: OkHttpClient by lazy {
        ApiClientFactory.create(
            allowCleartext = allowCleartext,
        )
    }

    val credentialVault: CredentialVault by lazy {
        credentialVaultOverride ?: CredentialVault.create(appContext)
    }

    private val serverUrlStore =
        serverUrlStoreOverride ?: ServerUrlStore(appContext, BuildConfig.API_BASE_URL)
    private val apiGraphRef by lazy {
        AtomicReference(createApiGraph(loadInitialBaseUrl()))
    }
    private val authRepositoryOverrideRef = AtomicReference<AuthRepository?>(null)
    private val serverConfigMutex = Mutex()

    val apiBaseUrl: String
        get() = apiGraphRef.get().baseUrl

    val api: CrossTalkApi
        get() = apiGraphRef.get().api

    val authRepository: AuthRepository
        get() = authRepositoryOverrideRef.get() ?: apiGraphRef.get().authRepository

    /**
     * Validates and persists the operator-selected server, then replaces the
     * API/auth graph used by subsequent login, session, and service requests.
     */
    suspend fun configureServer(rawServerUrl: String): AuthRepository = serverConfigMutex.withLock {
        val normalized = ServerEndpoint.normalize(rawServerUrl, allowCleartext)
        val current = apiGraphRef.get()
        authRepositoryOverrideRef.set(null)
        if (current.baseUrl == normalized) return@withLock current.authRepository
        val serviceState = audioServiceGateway.state.value
        require(!serviceState.userRequestedLive && !serviceState.isLiveOrConnecting) {
            "Stop live translation before changing servers"
        }

        current.api.setAccessToken(null)
        credentialVault.clear()
        val next = createApiGraph(normalized)
        withContext(Dispatchers.IO) {
            serverUrlStore.save(normalized)
        }
        apiGraphRef.set(next)
        next.authRepository
    }

    val lastSessionStore: LastSessionStore by lazy {
        LastSessionStore(appContext)
    }

    /**
     * Production factory: each connect/reconnect attempt constructs a fresh engine.
     * Not publicly mutable. Debug/test builds may install a factory via
     * [installRtcEngineFactoryForTests]; release rejects replacement.
     */
    private val rtcEngineFactoryRef =
        AtomicReference<() -> RtcEngine> {
            LibWebRtcEngine(
                appContext = appContext,
                httpClient = okHttpClient,
            )
        }

    val rtcEngineFactory: () -> RtcEngine
        get() = rtcEngineFactoryRef.get()

    /**
     * Test-only injection. Release builds throw — production cannot swap in a fake RTC engine.
     */
    fun installRtcEngineFactoryForTests(factory: () -> RtcEngine) {
        check(BuildConfig.DEBUG) { "RTC factory replacement is forbidden in release builds" }
        rtcEngineFactoryRef.set(factory)
    }

    fun installAuthRepositoryForTests(repository: AuthRepository) {
        check(BuildConfig.DEBUG) { "Auth repository replacement is forbidden in release builds" }
        authRepositoryOverrideRef.set(repository)
    }

    val audioServiceGateway: AudioServiceGateway by lazy {
        audioServiceGatewayOverride ?: BoundAudioServiceGateway(appContext)
    }

    fun applicationContext(): Context = appContext

    /**
     * Derive WSS/WS signaling base from REST API base URL.
     * https://host → wss://host ; http://host → ws://host
     */
    fun wsBaseUrl(): String = httpToWsBase(apiBaseUrl)

    private fun loadInitialBaseUrl(): String =
        runCatching {
            ServerEndpoint.normalize(serverUrlStore.read(), allowCleartext)
        }.getOrElse {
            ServerEndpoint.normalize(BuildConfig.API_BASE_URL, allowCleartext)
        }

    private fun createApiGraph(baseUrl: String): ApiGraph {
        TlsPolicy.assertSafeBaseUrl(baseUrl, allowCleartext = allowCleartext)
        val generatedApi = GeneratedApiAdapter(
            baseUrl = baseUrl,
            client = okHttpClient,
        )
        val repository = AuthRepository(
            api = generatedApi,
            vault = credentialVault,
            accessTokenSink = { token -> generatedApi.setAccessToken(token) },
        )
        return ApiGraph(
            baseUrl = baseUrl,
            api = generatedApi,
            authRepository = repository,
        )
    }

    private data class ApiGraph(
        val baseUrl: String,
        val api: GeneratedApiAdapter,
        val authRepository: AuthRepository,
    )

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
