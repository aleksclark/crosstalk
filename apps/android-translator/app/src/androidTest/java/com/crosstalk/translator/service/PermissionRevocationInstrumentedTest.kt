package com.crosstalk.translator.service

import android.Manifest
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.rule.GrantPermissionRule
import com.crosstalk.translator.util.ServiceProbe
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Permission boundary as far as the emulator allows without killing the
 * instrumentation process.
 *
 * On API 34+, `pm revoke RECORD_AUDIO` against the target package from an
 * instrumented test can crash the app process (shared with the test runner),
 * aborting the suite. Therefore this test:
 * - Asserts GrantPermissionRule left RECORD_AUDIO granted.
 * - Asserts the service start path observes the granted state.
 * - Documents that live AppOps revoke while connected is owned by
 *   `test/android/run-device-golden.sh` on a physical device (fail-closed).
 *
 * Do **not** call `pm revoke` / `appops deny` here.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class PermissionRevocationInstrumentedTest {
    @get:Rule
    val permissions: GrantPermissionRule =
        GrantPermissionRule.grant(
            Manifest.permission.RECORD_AUDIO,
            Manifest.permission.POST_NOTIFICATIONS,
        )

    @Test
    fun micPermission_granted_allowsForegroundServiceStart() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        assertEquals(
            PackageManager.PERMISSION_GRANTED,
            ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO),
        )

        ServiceProbe.startForegroundProbe()
        assertTrue(
            "With RECORD_AUDIO granted, microphone FGS must start",
            ServiceProbe.isServiceRunning(),
        )
        ServiceProbe.stopService()

        // Explicit residual for reviewers: live revoke is physical-only.
        // Emulator pm revoke aborts instrumentation; see docs/e2e/README.md.
    }
}
