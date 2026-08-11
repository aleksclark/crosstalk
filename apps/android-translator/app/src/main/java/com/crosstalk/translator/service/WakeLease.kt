package com.crosstalk.translator.service

import android.content.Context
import android.os.PowerManager
import com.crosstalk.translator.util.Clock
import com.crosstalk.translator.util.SystemClock
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

/**
 * Non-reference-counted partial wake lock with a hard 10-minute timeout.
 * Renewed by the health watchdog while a user-requested live session is active.
 * Release on every exit path; [isHeld] is testable.
 */
class WakeLease(
    context: Context,
    private val clock: Clock = SystemClock(),
    private val timeoutMs: Long = DEFAULT_TIMEOUT_MS,
    tag: String = DEFAULT_TAG,
) {
    private val appContext = context.applicationContext
    private val powerManager =
        appContext.getSystemService(Context.POWER_SERVICE) as PowerManager

    private val wakeLock: PowerManager.WakeLock =
        powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, tag).apply {
            setReferenceCounted(false)
        }

    private val held = AtomicBoolean(false)
    private val expiresAtMs = AtomicReference<Long?>(null)

    fun isHeld(): Boolean = held.get() && wakeLock.isHeld

    fun expiresAtEpochMs(): Long? = expiresAtMs.get()

    /**
     * Acquire or renew the lease. Safe to call repeatedly while live.
     */
    fun acquireOrRenew() {
        val now = clock.nowEpochMs()
        val until = now + timeoutMs
        expiresAtMs.set(until)
        if (!wakeLock.isHeld) {
            wakeLock.acquire(timeoutMs)
        } else {
            // Re-acquire with timeout to renew the platform timer.
            wakeLock.acquire(timeoutMs)
        }
        held.set(true)
    }

    /**
     * Drop the lease if the timeout has elapsed without renewal.
     * @return true if still held after the check.
     */
    fun checkTimeoutAndReleaseIfExpired(): Boolean {
        if (!held.get()) return false
        val until = expiresAtMs.get() ?: run {
            release()
            return false
        }
        if (clock.nowEpochMs() >= until) {
            release()
            return false
        }
        return isHeld()
    }

    fun release() {
        held.set(false)
        expiresAtMs.set(null)
        if (wakeLock.isHeld) {
            try {
                wakeLock.release()
            } catch (_: RuntimeException) {
                // Already released under race — ignore.
            }
        }
    }

    companion object {
        const val DEFAULT_TIMEOUT_MS: Long = 10 * 60_000L
        const val DEFAULT_TAG: String = "crosstalk:translator:wake"
    }
}
