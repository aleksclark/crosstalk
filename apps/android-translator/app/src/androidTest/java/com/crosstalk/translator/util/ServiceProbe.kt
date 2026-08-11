package com.crosstalk.translator.util

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import com.crosstalk.translator.service.ServiceState
import com.crosstalk.translator.service.TranslatorAudioService
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * Bind to [TranslatorAudioService] from instrumentation and sample state.
 * Does not force-stop the process.
 *
 * Note: [android.app.ActivityManager.getRunningServices] is unreliable / empty on
 * modern API levels even for the caller's UID, so service-liveness uses dumpsys
 * + bind probes instead.
 */
object ServiceProbe {
    fun packageName(): String =
        InstrumentationRegistry.getInstrumentation().targetContext.packageName

    fun isServiceRunning(context: Context = InstrumentationRegistry.getInstrumentation().targetContext): Boolean {
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val pkg = context.packageName
        val dump =
            device.executeShellCommand("dumpsys activity services $pkg")
                .orEmpty()
        if (dump.contains("TranslatorAudioService")) return true

        // Fallback: try a short bind without auto-create — only succeeds if alive.
        return runCatching {
            val binderRef = AtomicReference<TranslatorAudioService.LocalBinder?>()
            val latch = CountDownLatch(1)
            val conn =
                object : ServiceConnection {
                    override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
                        binderRef.set(service as? TranslatorAudioService.LocalBinder)
                        latch.countDown()
                    }

                    override fun onServiceDisconnected(name: ComponentName?) = Unit
                }
            val intent = Intent(context, TranslatorAudioService::class.java)
            val bound = context.bindService(intent, conn, 0)
            if (!bound) return@runCatching false
            try {
                if (!latch.await(2, TimeUnit.SECONDS)) return@runCatching false
                binderRef.get() != null
            } finally {
                runCatching { context.unbindService(conn) }
            }
        }.getOrDefault(false)
    }

    fun withBoundService(
        timeoutSec: Long = 10,
        block: (TranslatorAudioService.LocalBinder) -> Unit,
    ) {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val binderRef = AtomicReference<TranslatorAudioService.LocalBinder?>()
        val latch = CountDownLatch(1)
        val conn =
            object : ServiceConnection {
                override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
                    binderRef.set(service as TranslatorAudioService.LocalBinder)
                    latch.countDown()
                }

                override fun onServiceDisconnected(name: ComponentName?) = Unit
            }
        val intent = Intent(context, TranslatorAudioService::class.java)
        val bound = context.bindService(intent, conn, Context.BIND_AUTO_CREATE)
        check(bound) { "bindService returned false" }
        try {
            check(latch.await(timeoutSec, TimeUnit.SECONDS)) { "service bind timeout" }
            block(binderRef.get()!!)
        } finally {
            runCatching { context.unbindService(conn) }
        }
    }

    fun currentState(timeoutSec: Long = 10): ServiceState {
        var state: ServiceState = ServiceState.Idle
        withBoundService(timeoutSec) { binder ->
            state = binder.state.value
        }
        return state
    }

    fun awaitPhase(
        timeoutSec: Long = 30,
        predicate: (ServiceState) -> Boolean,
    ): ServiceState {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(timeoutSec)
        var last = ServiceState.Idle
        while (System.nanoTime() < deadline) {
            last = currentState(timeoutSec = 5)
            if (predicate(last)) return last
            Thread.sleep(250)
        }
        error("Timed out waiting for service phase; last=$last")
    }

    /** Start FGS without Join (Join needs auth/server and stopSelf on failure). */
    fun startForegroundProbe(
        context: Context = InstrumentationRegistry.getInstrumentation().targetContext,
    ) {
        // No action → ensureForeground only; stays up until Stop (no mint/auth).
        context.startForegroundService(Intent(context, TranslatorAudioService::class.java))
        val deadline = System.currentTimeMillis() + 8_000
        while (System.currentTimeMillis() < deadline) {
            if (isServiceRunning(context)) return
            Thread.sleep(250)
        }
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val dump = device.executeShellCommand("dumpsys activity services ${context.packageName}")
        error(
            "TranslatorAudioService did not appear after startForegroundService. dumpsys tail:\n" +
                dump.takeLast(1500),
        )
    }

    /**
     * Fire JOIN intent (may transition to Failed/stopSelf without server credentials).
     * Used only to prove the intent path is accepted; continuity tests use [startForegroundProbe].
     */
    fun startJoinProbe(
        sessionId: String,
        sessionName: String,
        context: Context = InstrumentationRegistry.getInstrumentation().targetContext,
    ) {
        val join =
            Intent(context, TranslatorAudioService::class.java).apply {
                action = TranslatorAudioService.ACTION_JOIN
                putExtra(TranslatorAudioService.EXTRA_SESSION_ID, sessionId)
                putExtra(TranslatorAudioService.EXTRA_SESSION_NAME, sessionName)
                putExtra(TranslatorAudioService.EXTRA_FEED_NAME, "Floor Feed")
                putExtra(TranslatorAudioService.EXTRA_BROADCAST_NAME, "English Broadcast")
            }
        context.startForegroundService(join)
        val deadline = System.currentTimeMillis() + 8_000
        while (System.currentTimeMillis() < deadline) {
            if (isServiceRunning(context)) return
            Thread.sleep(250)
        }
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val dump = device.executeShellCommand("dumpsys activity services ${context.packageName}")
        error(
            "TranslatorAudioService did not appear after JOIN startForegroundService. dumpsys tail:\n" +
                dump.takeLast(1500),
        )
    }

    fun stopService(context: Context = InstrumentationRegistry.getInstrumentation().targetContext) {
        context.startService(
            Intent(context, TranslatorAudioService::class.java).apply {
                action = TranslatorAudioService.ACTION_STOP
            },
        )
        Thread.sleep(1_000)
    }
}
