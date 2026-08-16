package com.crosstalk.translator.network

import com.crosstalk.translator.BuildConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ServerEndpointTest {
    @Test
    fun productionServerIsDefaultAndBareHostsNormalizeToHttps() {
        assertEquals("https://crosstalk-sfu.fly.dev", BuildConfig.API_BASE_URL)
        assertEquals(
            "https://translation.example",
            ServerEndpoint.normalize(" translation.example/ ", allowCleartext = false),
        )
    }

    @Test
    fun malformedServerIsReportedAsAnInputError() {
        val error = runCatching {
            ServerEndpoint.normalize("https://bad host", allowCleartext = false)
        }.exceptionOrNull()

        assertTrue(error is IllegalArgumentException)
    }

    @Test
    fun mixedCaseSchemeIsCanonicalizedForWebSocketDerivation() {
        assertEquals(
            "https://translation.example",
            ServerEndpoint.normalize("Https://Translation.Example/", allowCleartext = false),
        )
    }

    @Test
    fun invalidPortsAreRejectedBeforeCreatingApiClients() {
        listOf(0, 65_536, 99_999).forEach { port ->
            val error = runCatching {
                ServerEndpoint.normalize("https://translation.example:$port", allowCleartext = false)
            }.exceptionOrNull()

            assertTrue("port $port should be rejected", error is IllegalArgumentException)
        }
    }
}
