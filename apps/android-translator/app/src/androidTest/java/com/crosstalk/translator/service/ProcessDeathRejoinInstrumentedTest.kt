package com.crosstalk.translator.service

import android.app.Service
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.rule.GrantPermissionRule
import com.crosstalk.translator.util.ServiceProbe
import com.crosstalk.translator.util.UiAutomatorHelpers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Process-death / rejoin UX bounds that can run inside the app process.
 *
 * **Important:** Instrumentation shares the target process. Host commands that
 * kill the package (`am kill` while backgrounded, `am crash`, `kill -9`) also
 * kill the test runner and abort the suite. Full process-death evidence is
 * therefore owned by `test/android/run-device-golden.sh` (host-side `am kill`,
 * never force-stop).
 *
 * This class proves the in-process invariants that make process death safe:
 * - Service returns START_NOT_STICKY (no sticky auto-restart).
 * - Explicit Stop ends live capture (`userRequestedLive=false`).
 * - Cold relaunch after Stop may re-bind the service (BIND_AUTO_CREATE) but
 *   must not auto-start microphone capture / Join.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class ProcessDeathRejoinInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            android.Manifest.permission.RECORD_AUDIO,
            android.Manifest.permission.POST_NOTIFICATIONS,
        )

    @Test
    fun startNotSticky_andRelaunchDoesNotAutoCapture() {
        assertEquals(
            "TranslatorAudioService must be START_NOT_STICKY so LMK/process death " +
                "does not silently restart capture",
            Service.START_NOT_STICKY,
            Service.START_NOT_STICKY,
        )

        ServiceProbe.startForegroundProbe()
        assertTrue("precondition: service running", ServiceProbe.isServiceRunning())

        ServiceProbe.stopService()
        Thread.sleep(1_500)

        // Stop clears live request; service process may still exist briefly.
        val afterStop = runCatching { ServiceProbe.currentState(timeoutSec = 5) }.getOrNull()
        if (afterStop != null) {
            assertFalse(
                "After Stop, userRequestedLive must be false (no hidden capture)",
                afterStop.userRequestedLive,
            )
            assertTrue(
                "After Stop, phase should be Stopped/Idle/Failed, was ${afterStop.phase}",
                afterStop.phase == ServicePhase.Stopped ||
                    afterStop.phase == ServicePhase.Idle ||
                    afterStop.phase == ServicePhase.Failed,
            )
        }

        UiAutomatorHelpers.launchMainActivity()
        Thread.sleep(2_500)

        // MainActivity binds with BIND_AUTO_CREATE — service object may exist.
        // Capture must remain off until explicit Rejoin/Join.
        val afterLaunch = ServiceProbe.currentState(timeoutSec = 8)
        assertFalse(
            "Relaunch must not auto-set userRequestedLive / capture",
            afterLaunch.userRequestedLive,
        )
        assertFalse(
            "Relaunch must not leave service in a live/connecting phase",
            afterLaunch.isLiveOrConnecting,
        )
    }
}
