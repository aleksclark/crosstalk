package com.crosstalk.translator.auth

import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.AuthTokens
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.GeneratedApiAdapter
import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.contract.SessionSummary
import com.crosstalk.translator.network.ApiClientFactory
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.util.concurrent.atomic.AtomicInteger

class AuthRepositoryTest {
    private lateinit var server: MockWebServer
    private lateinit var vault: CredentialVault
    private lateinit var adapter: GeneratedApiAdapter
    private lateinit var repo: AuthRepository

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        vault = createTestCredentialVault()
        val client = ApiClientFactory.create(allowCleartext = true)
        adapter = GeneratedApiAdapter(
            baseUrl = server.url("/").toString().trimEnd('/'),
            client = client,
        )
        repo = AuthRepository(
            api = adapter,
            vault = vault,
            accessTokenSink = { adapter.setAccessToken(it) },
        )
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun loginStoresEncryptedRefreshAndMemoryAccessOnly() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"access-jwt-1","refresh_token":"refresh-opaque-1"}""")
                .addHeader("Content-Type", "application/json"),
        )

        val state = repo.login("translator1", "s3cret")
        assertTrue(state is AuthState.SignedIn)
        assertEquals("translator1", (state as AuthState.SignedIn).username)
        assertEquals("refresh-opaque-1", vault.readRefreshToken())

        val envelope = vault.readEnvelope()!!
        assertFalse(envelope.ciphertextBase64.contains("refresh-opaque-1"))
        assertFalse(envelope.ciphertextBase64.contains("access-jwt-1"))

        val recorded = server.takeRequest()
        assertEquals("/api/auth/login", recorded.path)
        assertTrue(recorded.body.readUtf8().contains("translator1"))
        assertNull(recorded.requestUrl?.query)
    }

    @Test
    fun restoreSessionRefreshesAndRotatesToken() = runBlocking {
        vault.saveRefreshToken("refresh-old")
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"access-new","refresh_token":"refresh-new"}""")
                .addHeader("Content-Type", "application/json"),
        )

        val state = repo.restoreSession()
        assertTrue(state is AuthState.SignedIn)
        assertEquals("refresh-new", vault.readRefreshToken())

        val req = server.takeRequest()
        assertEquals("/api/auth/refresh", req.path)
        assertTrue(req.body.readUtf8().contains("refresh-old"))
    }

    @Test
    fun restoreWithRevokedRefreshClearsVault() = runBlocking {
        vault.saveRefreshToken("revoked")
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"title":"unauthorized"}"""))

        val state = repo.restoreSession()
        assertTrue(state is AuthState.SignedOut)
        assertNull(vault.readRefreshToken())
    }

    @Test
    fun concurrentRefreshIsDeduped() = runBlocking {
        val api = CountingRefreshApi(delayMs = 80)
        val countingRepo = AuthRepository(
            api = api,
            vault = vault,
            accessTokenSink = {},
        )
        vault.saveRefreshToken("shared-refresh")

        val results = (1..5).map {
            async { countingRepo.restoreSession() }
        }.awaitAll()

        assertTrue(results.all { it is AuthState.SignedIn })
        assertEquals(1, api.refreshCalls.get())
        assertEquals("rotated-refresh", vault.readRefreshToken())
    }

    @Test
    fun unauthorizedReadTriggersSingleRefreshAndReplay() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"a1","refresh_token":"r1"}""")
                .addHeader("Content-Type", "application/json"),
        )
        repo.login("u", "p")
        server.takeRequest()

        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"title":"expired"}"""))
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"a2","refresh_token":"r2"}""")
                .addHeader("Content-Type", "application/json"),
        )
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """{"data":[{"id":"01SESSION","name":"Sunday Spanish","description":"","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}]}""",
                )
                .addHeader("Content-Type", "application/json"),
        )

        val sessions = repo.listAssignedSessions()
        assertEquals(1, sessions.size)
        assertEquals("Sunday Spanish", sessions[0].name)
        assertEquals("01SESSION", sessions[0].id)

        assertEquals("/api/sessions", server.takeRequest().path)
        assertEquals("/api/auth/refresh", server.takeRequest().path)
        assertEquals("/api/sessions", server.takeRequest().path)
        assertEquals("r2", vault.readRefreshToken())
    }

    @Test
    fun logoutClearsEvenWhenNetworkFails() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"a1","refresh_token":"r1"}""")
                .addHeader("Content-Type", "application/json"),
        )
        repo.login("u", "p")
        server.takeRequest()

        server.enqueue(MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START))

        repo.logout()
        assertNull(vault.readRefreshToken())
        assertTrue(repo.authState.value is AuthState.SignedOut)
    }

    @Test
    fun mintMediaTicketSendsOnlySessionAndRole() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody("""{"access_token":"a1","refresh_token":"r1"}""")
                .addHeader("Content-Type", "application/json"),
        )
        repo.login("u", "p")
        server.takeRequest()

        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setBody(
                    """
                    {
                      "token":"media-ticket-xyz",
                      "session_id":"01SESSION",
                      "role":"translator",
                      "expires_at":"2030-01-01T00:00:00Z",
                      "owner_generation":1,
                      "produce_channel_ids":["01BROADCAST"],
                      "listen_channel_ids":["01FEED"]
                    }
                    """.trimIndent(),
                )
                .addHeader("Content-Type", "application/json"),
        )

        val ticket = repo.mintMediaTicket("01SESSION")
        assertEquals("media-ticket-xyz", ticket.token)
        assertEquals("translator", ticket.role)
        assertEquals("01SESSION", ticket.sessionId)
        assertTrue(ticket.expiresAtEpochMs > 0)
        assertEquals(listOf("01BROADCAST"), ticket.produceChannelIds)
        assertEquals(listOf("01FEED"), ticket.listenChannelIds)

        val req = server.takeRequest()
        assertEquals("/api/webrtc/token", req.path)
        val body = req.body.readUtf8()
        assertTrue(body.contains("\"session_id\""))
        assertTrue(body.contains("\"role\""))
        assertTrue(body.contains("translator"))
        // Client must not request channel capability narrowing.
        assertFalse(body.contains("01BROADCAST"))
        assertFalse(body.contains("01FEED"))
        assertFalse(body.contains("\"produce\":["))
        assertFalse(body.contains("\"listen\":["))
        assertTrue(req.getHeader("Authorization")!!.startsWith("Bearer "))
    }

    @Test
    fun loginFailureDoesNotPersistPasswordOrTokens() = runBlocking {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"title":"bad"}"""))
        try {
            repo.login("u", "bad-password")
            fail("expected unauthorized")
        } catch (_: ApiException.Unauthorized) {
            // expected
        }
        assertNull(vault.readRefreshToken())
        assertTrue(repo.authState.value is AuthState.SignedOut)
    }

    private class CountingRefreshApi(
        private val delayMs: Long,
    ) : CrossTalkApi {
        val refreshCalls = AtomicInteger(0)

        override suspend fun login(username: String, password: String): AuthTokens =
            error("not used")

        override suspend fun refresh(refreshToken: String): AuthTokens {
            refreshCalls.incrementAndGet()
            delay(delayMs)
            return AuthTokens(
                accessToken = "access-from-refresh",
                refreshToken = "rotated-refresh",
            )
        }

        override suspend fun logout(refreshToken: String) = Unit
        override suspend fun listSessions(): List<SessionSummary> = emptyList()
        override suspend fun getSession(sessionId: String): SessionSummary = error("not used")
        override suspend fun listChannels(sessionId: String): List<ChannelInfo> = emptyList()
        override suspend fun mintMediaTicket(sessionId: String): MediaTicket =
            error("not used")
    }
}
