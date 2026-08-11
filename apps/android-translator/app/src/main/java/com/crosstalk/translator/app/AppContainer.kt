package com.crosstalk.translator.app

import android.content.Context
import com.crosstalk.translator.BuildConfig
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.network.ApiClientFactory
import com.crosstalk.translator.util.SystemClock
import okhttp3.OkHttpClient

/**
 * Manual constructor-injection graph for the single-module app.
 * Later phases wire auth, RTC, and service gateways here.
 */
class AppContainer(
    context: Context,
) {
    private val appContext = context.applicationContext

    val clock = SystemClock()

    val okHttpClient: OkHttpClient by lazy {
        ApiClientFactory.create(
            allowCleartext = BuildConfig.ALLOW_CLEARTEXT,
        )
    }

    /**
     * REST surface. Phase 2 binds the generated OpenAPI adapter.
     * Phase 1 exposes a failing placeholder so callers do not silently no-op.
     */
    val api: CrossTalkApi by lazy {
        object : CrossTalkApi {
            private fun notImplemented(): Nothing =
                error("CrossTalkApi adapter is not wired until Phase 2")

            override suspend fun login(username: String, password: String) = notImplemented()
            override suspend fun refresh(refreshToken: String) = notImplemented()
            override suspend fun logout(refreshToken: String) = notImplemented()
            override suspend fun listSessions() = notImplemented()
            override suspend fun getSession(sessionId: String) = notImplemented()
            override suspend fun listChannels(sessionId: String) = notImplemented()
            override suspend fun mintMediaTicket(sessionId: String, role: String) = notImplemented()
        }
    }

    val apiBaseUrl: String = BuildConfig.API_BASE_URL

    fun applicationContext(): Context = appContext
}
