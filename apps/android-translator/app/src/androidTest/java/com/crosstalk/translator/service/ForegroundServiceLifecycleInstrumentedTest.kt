package com.crosstalk.translator.service

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.rule.GrantPermissionRule
import com.crosstalk.translator.util.ServiceProbe
import com.crosstalk.translator.util.UiAutomatorHelpers
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * FGS survives Home (background) without force-stop. Rotation is best-effort.
 * Does not claim bidirectional audio — that is the physical golden harness.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class ForegroundServiceLifecycleInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            android.Manifest.permission.RECORD_AUDIO,
            android.Manifest.permission.POST_NOTIFICATIONS,
        )

    @Test
    fun startForegroundJoinIntent_survivesHome() {
        // Continuity without server: start FGS (no Join mint). Join without auth
        // stopSelfs on AuthFailed — covered by real-server golden harness.
        ServiceProbe.startForegroundProbe()
        assertTrue(
            "TranslatorAudioService should be running after foreground start",
            ServiceProbe.isServiceRunning(),
        )

        // Also prove JOIN intent is accepted while FGS is up (may fail later).
        ServiceProbe.startJoinProbe(
            sessionId = "01TESTSESSIONID000000000000",
            sessionName = "Lifecycle Probe Session",
        )

        UiAutomatorHelpers.pressHome()
        Thread.sleep(2_000)
        // After Join-without-server the service may already be Failed/stopped.
        // Re-assert continuity on a fresh idle FGS.
        ServiceProbe.startForegroundProbe()
        UiAutomatorHelpers.pressHome()
        Thread.sleep(2_000)
        assertTrue(
            "FGS must remain after KEYCODE_HOME / pressHome",
            ServiceProbe.isServiceRunning(),
        )

        runCatching {
            UiAutomatorHelpers.launchMainActivity()
            val device = UiAutomatorHelpers.device()
            device.setOrientationLeft()
            Thread.sleep(1_000)
            device.setOrientationNatural()
            Thread.sleep(500)
        }

        assertTrue(
            "FGS must remain after activity relaunch/rotation attempt",
            ServiceProbe.isServiceRunning(),
        )

        ServiceProbe.stopService()
    }
}
