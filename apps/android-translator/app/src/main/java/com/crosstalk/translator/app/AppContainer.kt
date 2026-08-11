package com.crosstalk.translator.app

import android.content.Context
import com.crosstalk.translator.BuildConfig
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.CredentialVault
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.GeneratedApiAdapter
import com.crosstalk.translator.network.ApiClientFactory
import com.crosstalk.translator.network.TlsPolicy
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

    fun applicationContext(): Context = appContext
}
