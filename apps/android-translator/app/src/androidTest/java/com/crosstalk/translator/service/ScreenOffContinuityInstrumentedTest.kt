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
 * Screen-off continuity smoke: service remains after KEYCODE_SLEEP simulation.
 *
 * Full 10-minute bidirectional spectral proof is owned by
 * `test/android/run-device-golden.sh` on a physical device.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class ScreenOffContinuityInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            android.Manifest.permission.RECORD_AUDIO,
            android.Manifest.permission.POST_NOTIFICATIONS,
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

        UiAutomatorHelpers.wakeDevice()
        Thread.sleep(1_000)
        assertTrue(
            "FGS must remain after WAKEUP",
            ServiceProbe.isServiceRunning(),
        )

        ServiceProbe.stopService()
    }
}
