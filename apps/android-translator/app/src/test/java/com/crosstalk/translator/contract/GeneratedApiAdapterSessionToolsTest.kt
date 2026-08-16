package com.crosstalk.translator.contract

import com.crosstalk.translator.network.ApiClientFactory
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class GeneratedApiAdapterSessionToolsTest {
    private lateinit var server: MockWebServer
    private lateinit var adapter: GeneratedApiAdapter

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        adapter = GeneratedApiAdapter(
            baseUrl = server.url("/").toString().trimEnd('/'),
            client = ApiClientFactory.create(allowCleartext = true),
        )
        adapter.setAccessToken("access-token")
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun mapsBroadcastSourcesAndMixEndpoints() = runTest {
        enqueueJson("""{"broadcast_token":"broadcast-token","url":"/broadcast/broadcast-token"}""")
        val link = adapter.getBroadcastLink("session-1")
        assertEquals("broadcast-token", link.token)
        assertEquals("/api/sessions/session-1/broadcast-url", server.takeRequest().path)

        enqueueJson(
            """{"data":[{"connected":true,"first_seen":"2026-08-16T00:00:00Z","id":"source-1","last_seen":"2026-08-16T00:00:01Z","name":"Booth mic","origin":"abc","session_id":"session-1"}]}""",
        )
        val source = adapter.listSources("session-1").single()
        assertEquals("Booth mic", source.name)
        assertTrue(source.connected)
        assertEquals("/api/sessions/session-1/sources", server.takeRequest().path)

        enqueueJson(
            """{"data":[{"channel_id":"channel-1","id":"mix-1","level":1.0,"muted":false,"source_id":"source-1"}]}""",
        )
        val mix = adapter.getMix("session-1", "channel-1").single()
        assertEquals("source-1", mix.sourceId)
        assertEquals("/api/sessions/session-1/channels/channel-1/mix", server.takeRequest().path)

        enqueueJson(
            """{"data":[{"channel_id":"channel-1","id":"mix-1","level":2.0,"muted":true,"source_id":"source-1"}]}""",
        )
        val updated = adapter.updateMix(
            sessionId = "session-1",
            channelId = "channel-1",
            entries = listOf(mix.copy(level = 3.0, muted = true)),
        ).single()
        assertEquals(2.0, updated.level, 0.0)
        assertTrue(updated.muted)
        val updateRequest = server.takeRequest()
        assertEquals("PUT", updateRequest.method)
        assertEquals("Bearer access-token", updateRequest.getHeader("Authorization"))
        val body = updateRequest.body.readUtf8()
        assertTrue(body.contains("\"source_id\":\"source-1\""))
        assertTrue(body.contains("\"level\":2.0"))
        assertTrue(body.contains("\"muted\":true"))
    }

    private fun enqueueJson(body: String) {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json")
                .setBody(body),
        )
    }
}
