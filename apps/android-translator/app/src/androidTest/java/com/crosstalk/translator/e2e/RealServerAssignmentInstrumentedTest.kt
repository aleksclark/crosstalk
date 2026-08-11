package com.crosstalk.translator.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import com.crosstalk.translator.util.AndroidTestEnv
import com.crosstalk.translator.util.RealServerClient
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Real CrossTalk assignment list against a live server.
 *
 * Requires env/instrumentation args:
 * - CROSSTALK_BASE_URL
 * - CROSSTALK_TRANSLATOR_USER / CROSSTALK_TRANSLATOR_PASSWORD
 *
 * When unset, assumeTrue skips with a clear reason so CI static jobs stay green
 * without fake-passing real-server proof. The device golden harness sets these.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class RealServerAssignmentInstrumentedTest {
    private var baseUrl: String? = null

    @Before
    fun requireServer() {
        baseUrl = AndroidTestEnv.baseUrl()
        assumeTrue(AndroidTestEnv.ignoreReasonNoServer(), AndroidTestEnv.realServerConfigured())
        val client = RealServerClient(baseUrl!!)
        assumeTrue(
            "CROSSTALK_BASE_URL set but server not reachable at ${baseUrl/* redacted path only */}",
            client.reachable(),
        )
    }

    @Test
    fun translatorSeesOnlyAssignedSessions() {
        val user = AndroidTestEnv.translatorUser()
        val pass = AndroidTestEnv.translatorPassword()
        assumeTrue(
            "Requires CROSSTALK_TRANSLATOR_USER and CROSSTALK_TRANSLATOR_PASSWORD",
            !user.isNullOrBlank() && !pass.isNullOrBlank(),
        )

        val client = RealServerClient(baseUrl!!)
        val tokens = client.login(user!!, pass!!)
        val sessions = client.listSessions(tokens.accessToken)
        // Seed harness always assigns at least one named session.
        assertTrue(
            "expected at least one assigned session from seed harness",
            sessions.isNotEmpty(),
        )
        sessions.forEach { row ->
            assertTrue(row.id.isNotBlank())
            assertTrue("session name must be human-readable primary", row.name.isNotBlank())
        }

        val expectedId = AndroidTestEnv.sessionId()
        if (!expectedId.isNullOrBlank()) {
            assertTrue(
                "seeded session id must appear in assignment list",
                sessions.any { it.id == expectedId },
            )
        }
    }
}
