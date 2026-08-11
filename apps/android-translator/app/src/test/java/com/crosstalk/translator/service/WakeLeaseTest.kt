package com.crosstalk.translator.service

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.crosstalk.translator.util.Clock
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class WakeLeaseTest {

    private lateinit var context: Context
    private lateinit var fakeClock: FakeClock

    @Before
    fun setUp() {
        context = ApplicationProvider.getApplicationContext()
        fakeClock = FakeClock(1_000L)
    }

    @Test
    fun acquireHoldsAndReleaseClears() {
        val lease = WakeLease(context, clock = fakeClock, timeoutMs = 60_000L)
        assertFalse(lease.isHeld())
        lease.acquireOrRenew()
        assertTrue(lease.isHeld())
        lease.release()
        assertFalse(lease.isHeld())
    }

    @Test
    fun renewExtendsTimeout() {
        val lease = WakeLease(context, clock = fakeClock, timeoutMs = 10_000L)
        lease.acquireOrRenew()
        fakeClock.now = 9_000L
        assertTrue(lease.checkTimeoutAndReleaseIfExpired())
        lease.acquireOrRenew() // renew at t=9s → expires 19s
        fakeClock.now = 15_000L
        assertTrue(lease.checkTimeoutAndReleaseIfExpired())
        fakeClock.now = 20_000L
        assertFalse(lease.checkTimeoutAndReleaseIfExpired())
        assertFalse(lease.isHeld())
    }

    @Test
    fun timeoutReleasesWithoutRenew() {
        val lease = WakeLease(context, clock = fakeClock, timeoutMs = 5_000L)
        lease.acquireOrRenew()
        fakeClock.now = 6_000L
        assertFalse(lease.checkTimeoutAndReleaseIfExpired())
        assertFalse(lease.isHeld())
    }

    @Test
    fun doubleReleaseIsSafe() {
        val lease = WakeLease(context, clock = fakeClock)
        lease.acquireOrRenew()
        lease.release()
        lease.release()
        assertFalse(lease.isHeld())
    }

    @Test
    fun noLeakAfterRelease() {
        val lease = WakeLease(context, clock = fakeClock, timeoutMs = 60_000L)
        lease.acquireOrRenew()
        assertNotNull(lease.expiresAtEpochMs())
        assertTrue(lease.isHeld())
        lease.release()
        assertFalse(lease.isHeld())
        // Second instance should also release cleanly (no static leak).
        val lease2 = WakeLease(context, clock = fakeClock, timeoutMs = 60_000L)
        lease2.acquireOrRenew()
        lease2.release()
        assertFalse(lease2.isHeld())
    }

    private class FakeClock(var now: Long) : Clock {
        override fun nowEpochMs(): Long = now
    }
}
