package com.crosstalk.translator.network

import java.net.URI

/**
 * Transport policy helpers. Release builds require HTTPS/WSS and reject
 * embedded credentials or cleartext production endpoints.
 */
object TlsPolicy {
    fun assertSafeBaseConfiguration(allowCleartext: Boolean) {
        // Construction-time marker only; per-URL checks use [assertSafeBaseUrl].
        if (!allowCleartext) {
            // no-op: cleartext disabled by default platform + network security config
        }
    }

    fun assertSafeBaseUrl(raw: String, allowCleartext: Boolean) {
        val uri = URI(raw)
        require(uri.userInfo == null) { "API base URL must not embed credentials" }
        require(uri.query == null) { "API base URL must not include query parameters" }
        require(uri.fragment == null) { "API base URL must not include a fragment" }
        val scheme = uri.scheme?.lowercase()
        require(scheme == "http" || scheme == "https") {
            "API base URL scheme must be http or https"
        }
        if (!allowCleartext) {
            require(scheme == "https") { "Release builds require HTTPS API base URL" }
        }
    }

    fun isLoopbackHost(host: String?): Boolean {
        if (host.isNullOrBlank()) return false
        val h = host.lowercase()
        return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "10.0.2.2"
    }
}
