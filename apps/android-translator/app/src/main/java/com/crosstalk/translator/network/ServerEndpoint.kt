package com.crosstalk.translator.network

import java.net.URI

/** Normalizes and validates the operator-selected CrossTalk server origin. */
object ServerEndpoint {
    const val PRODUCTION_BASE_URL = "https://crosstalk-sfu.fly.dev"

    fun normalize(raw: String, allowCleartext: Boolean): String {
        try {
            val value = raw.trim()
            require(value.isNotEmpty()) { "Enter a server URL" }
            val withScheme = if ("://" in value) value else "https://$value"
            val normalized = withScheme.trimEnd('/')
            TlsPolicy.assertSafeBaseUrl(normalized, allowCleartext = allowCleartext)

            val uri = URI(normalized)
            require(!uri.host.isNullOrBlank()) { "Enter a valid server URL" }
            require(uri.path.isNullOrEmpty()) { "Server URL must not include a path" }
            require(uri.port == -1 || uri.port in 1..65_535) {
                "Server URL port must be between 1 and 65535"
            }
            return URI(
                uri.scheme.lowercase(),
                null,
                uri.host.lowercase(),
                uri.port,
                null,
                null,
                null,
            ).toString()
        } catch (error: IllegalArgumentException) {
            throw error
        } catch (error: Exception) {
            throw IllegalArgumentException("Enter a valid server URL", error)
        }
    }
}
