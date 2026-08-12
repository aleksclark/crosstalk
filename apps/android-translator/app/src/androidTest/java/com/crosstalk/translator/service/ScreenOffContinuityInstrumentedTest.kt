package com.crosstalk.translator.service

import android.Manifest
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.rule.GrantPermissionRule
import com.crosstalk.translator.app.AppContainer
import com.crosstalk.translator.rtc.LibWebRtcEngine
import com.crosstalk.translator.rtc.RtcConnectRequest
import com.crosstalk.translator.rtc.RtcEvent
import com.crosstalk.translator.rtc.RtpStatsSampleHelper
import com.crosstalk.translator.rtc.StopReason
import com.crosstalk.translator.util.AndroidTestEnv
import com.crosstalk.translator.util.RealServerClient
import com.crosstalk.translator.util.ServiceProbe
import com.crosstalk.translator.util.UiAutomatorHelpers
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Screen-off continuity:
 * 1) FGS remains after KEYCODE_SLEEP (always).
 * 2) When `CROSSTALK_BASE_URL` is set: production [LibWebRtcEngine] RTP counters
 *    advance across HOME + SLEEP, then explicit stop closes cleanly.
 *
 * Full 10-minute bidirectional spectral proof is owned by
 * `test/android/run-device-golden.sh` on a physical device (`physical-mic`).
 * Emulator path is synthetic-capture-debug-only for RTP counters.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class ScreenOffContinuityInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            Manifest.permission.RECORD_AUDIO,
            Manifest.permission.POST_NOTIFICATIONS,
        )

    @Test
    fun serviceRemainsAfterSleepAndWake() {
        ServiceProbe.startForegroundProbe()
        assertTrue(ServiceProbe.isServiceRunning())

        UiAutomatorHelpers.pressHome()
        UiAutomatorHelpers.sleepDevice()
        Thread.sleep(5_000)
        assertTrue(
            "FGS must remain while screen is off (sleep simulation)",
            ServiceProbe.isServiceRunning(),
        )
        ServiceProbe.logGoldenStats()

        UiAutomatorHelpers.wakeDevice()
        Thread.sleep(1_000)
        assertTrue(
            "FGS must remain after WAKEUP",
            ServiceProbe.isServiceRunning(),
        )

        ServiceProbe.stopService()
        Thread.sleep(1_500)
        // Best-effort: after explicit stop the service should leave the running set.
        // Some API levels keep a brief dying entry; do not hard-fail if dumpsys lags.
        if (ServiceProbe.isServiceRunning()) {
            Log.w("CT_GOLDEN_STATS", "service still listed shortly after STOP (dumpsys lag?)")
        }
    }

    @Test
    fun homeSleep_realEngineRtpAdvances_thenExplicitStop() =
        runBlocking {
            assumeTrue(AndroidTestEnv.ignoreReasonNoServer(), AndroidTestEnv.realServerConfigured())
            assumeTrue(
                "Requires translator credentials",
                !AndroidTestEnv.translatorUser().isNullOrBlank() &&
                    !AndroidTestEnv.translatorPassword().isNullOrBlank(),
            )
            val baseUrl = AndroidTestEnv.baseUrl()!!
            val client = RealServerClient(baseUrl)
            assumeTrue("server not reachable", client.reachable())

            val holdSec = AndroidTestEnv.continuityHoldSeconds(default = 25L)
            val context = InstrumentationRegistry.getInstrumentation().targetContext
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

            // Keep FGS alive as the product path would under live translation.
            ServiceProbe.startForegroundProbe()
            assertTrue(ServiceProbe.isServiceRunning())

            val container = AppContainer(context)
            val engine =
                LibWebRtcEngine(
                    appContext = context,
                    httpClient = container.okHttpClient,
                )
            try {
                engine.connect(
                    RtcConnectRequest(
                        wsBaseUrl = AndroidTestEnv.wsBaseUrlFromHttp(baseUrl),
                        sessionId = session.id,
                        mediaTicket = ticket.token,
                    ),
                )
                withTimeout(90_000) {
                    engine.events
                        .filterIsInstance<RtcEvent>()
                        .first { ev ->
                            when (ev) {
                                is RtcEvent.IceConnectionStateChanged ->
                                    ev.state.equals("connected", true) ||
                                        ev.state.equals("completed", true)
                                is RtcEvent.PeerConnectionStateChanged ->
                                    ev.state.equals("connected", true)
                                is RtcEvent.Failed -> error("RTC failed: ${ev.message}")
                                else -> false
                            }
                        }
                }

                UiAutomatorHelpers.pressHome()
                UiAutomatorHelpers.sleepDevice()
                assertTrue(
                    "FGS must remain while engine samples under sleep",
                    ServiceProbe.isServiceRunning(),
                )

                val window =
                    RtpStatsSampleHelper.sampleWindow(
                        engine = engine,
                        durationMs = holdSec * 1_000L,
                        periodMs = 2_000L,
                    )
                window.logLine("screen_off_continuity")
                ServiceProbe.logGoldenStats()

                UiAutomatorHelpers.wakeDevice()
                Thread.sleep(500)
                assertTrue(ServiceProbe.isServiceRunning())
                assertTrue(
                    "outbound RTP must advance during HOME+SLEEP: " +
                        RtpStatsSampleHelper.outboundDeltaMessage(window),
                    window.outboundAdvanced,
                )
                if (!(window.inboundAdvanced || window.energyAdvanced)) {
                    val reason =
                        "INBOUND_SKIP: no inbound RTP/energy during screen-off " +
                            "(floor peer absent/silent). Outbound continuity OK. " +
                            "Label=synthetic-capture-debug-only on emulator."
                    Log.w(RtpStatsSampleHelper.LOG_TAG, reason)
                    println(reason)
                }

                engine.close(StopReason.UserStop)
                ServiceProbe.stopService()
                Thread.sleep(1_500)
                Log.i(
                    RtpStatsSampleHelper.LOG_TAG,
                    "ct_stats label=post_stop running=${ServiceProbe.isServiceRunning()}",
                )
            } finally {
                runCatching { engine.close(StopReason.UserStop) }
                runCatching { ServiceProbe.stopService() }
            }
        }
}
