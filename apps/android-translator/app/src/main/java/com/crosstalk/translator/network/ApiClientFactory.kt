package com.crosstalk.translator.network

import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

object ApiClientFactory {
    fun create(
        allowCleartext: Boolean = false,
        eventListener: RedactingEventListener = RedactingEventListener(),
    ): OkHttpClient {
        // Phase 1: establish client defaults. Certificate pinning / custom
        // trust managers arrive with deployment configuration later.
        TlsPolicy.assertSafeBaseConfiguration(allowCleartext = allowCleartext)
        return OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .callTimeout(45, TimeUnit.SECONDS)
            .eventListener(eventListener)
            .build()
    }
}
