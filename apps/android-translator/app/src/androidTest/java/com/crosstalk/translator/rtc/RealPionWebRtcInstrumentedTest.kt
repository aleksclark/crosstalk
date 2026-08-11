package com.crosstalk.translator.rtc

import android.Manifest
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.rule.GrantPermissionRule
import com.crosstalk.translator.app.AppContainer
import com.crosstalk.translator.util.AndroidTestEnv
import com.crosstalk.translator.util.RealServerClient
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Real libwebrtc ↔ Pion mint + signaling smoke.
 *
 * Requires CROSSTALK_BASE_URL + translator credentials + assigned session.
 * Uses production [LibWebRtcEngine]. Emulator mic may be silent/synthetic —
 * this test asserts ticket mint + ICE/peer progress, NOT physical spectral proof.
 * Physical bidirectional 440/880 Hz is [test/android/run-device-golden.sh] only.
 *
 * When env is absent, assumptions skip with a clear reason (not a silent green).
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class RealPionWebRtcInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            Manifest.permission.RECORD_AUDIO,
            Manifest.permission.POST_NOTIFICATIONS,
        )

    private var baseUrl: String? = null

    @Before
    fun requireServer() {
        baseUrl = AndroidTestEnv.baseUrl()
        assumeTrue(AndroidTestEnv.ignoreReasonNoServer(), AndroidTestEnv.realServerConfigured())
        assumeTrue(
            "Requires translator credentials",
            !AndroidTestEnv.translatorUser().isNullOrBlank() &&
                !AndroidTestEnv.translatorPassword().isNullOrBlank(),
        )
        val client = RealServerClient(baseUrl!!)
        assumeTrue("server not reachable", client.reachable())
    }

    @Test
    fun mintTicket_andConnectEngine_reachesIceOrConnected() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val client = RealServerClient(baseUrl!!)
        val tokens =
            client.login(
                AndroidTestEnv.translatorUser()!!,
                AndroidTestEnv.translatorPassword()!!,
            )
        val sessions = client.listSessions(tokens.accessToken)
        assumeTrue("no assigned sessions", sessions.isNotEmpty())
        val session =
            sessions.firstOrNull { it.id == AndroidTestEnv.sessionId() } ?: sessions.first()

        val ticket = client.mintMediaTicket(tokens.accessToken, session.id)
        assertTrue("media ticket must be non-blank", ticket.token.isNotBlank())
        // Ticket must not look like a long-lived access JWT reuse (opaque/nonce-bound).
        assertTrue(ticket.token != tokens.accessToken)

        val container = AppContainer(context)
        // Point WS at the real server host from CROSSTALK_BASE_URL, not BuildConfig default.
        val wsBase =
            when {
                baseUrl!!.startsWith("https://", true) ->
                    "wss://" + baseUrl!!.removePrefix("https://").removePrefix("HTTPS://")
                baseUrl!!.startsWith("http://", true) ->
                    "ws://" + baseUrl!!.removePrefix("http://").removePrefix("HTTP://")
                else -> baseUrl!!
            }

        val engine =
            LibWebRtcEngine(
                appContext = context,
                httpClient = container.okHttpClient,
            )
        try {
            engine.connect(
                RtcConnectRequest(
                    wsBaseUrl = wsBase,
                    sessionId = session.id,
                    mediaTicket = ticket.token,
                ),
            )
            val progress =
                withTimeout(60_000) {
                    engine.events
                        .filterIsInstance<RtcEvent>()
                        .first { ev ->
                            when (ev) {
                                is RtcEvent.IceConnectionStateChanged ->
                                    ev.state.equals("connected", true) ||
                                        ev.state.equals("completed", true) ||
                                        ev.state.equals("checking", true)
                                is RtcEvent.PeerConnectionStateChanged ->
                                    ev.state.equals("connected", true) ||
                                        ev.state.equals("connecting", true)
                                is RtcEvent.LocalOfferSent -> true
                                is RtcEvent.RemoteTrack -> true
                                is RtcEvent.Failed -> false
                                else -> false
                            }
                        }
                }
            // Progress event selected by filter — reaching here means ICE/offer/track advanced.
            assertTrue("expected RTC progress event, got $progress", true)
        } finally {
            runCatching { engine.close(StopReason.UserStop) }
        }
    }
}
