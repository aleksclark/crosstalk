package com.crosstalk.translator.util

import android.Manifest
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.test.core.app.ApplicationProvider
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.rule.GrantPermissionRule
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assert.assertTrue
import org.junit.rules.RuleChain
import org.junit.rules.TestRule

object UiAutomatorHelpers {
    fun device(): UiDevice =
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())

    fun launchMainActivity() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val intent =
            context.packageManager.getLaunchIntentForPackage(context.packageName)
                ?: Intent().setClassName(context.packageName, "com.crosstalk.translator.MainActivity")
        intent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TASK or Intent.FLAG_ACTIVITY_NEW_TASK)
        context.startActivity(intent)
        device().wait(Until.hasObject(By.pkg(context.packageName).depth(0)), 10_000)
    }

    fun waitForTestTag(tag: String, timeoutMs: Long = 15_000): Boolean {
        // Compose test tags surface as resource-id-like descriptors on some APIs;
        // also match by description/text fallbacks used in product UI.
        val d = device()
        val byRes = d.wait(Until.hasObject(By.res(tag)), timeoutMs / 3)
        if (byRes) return true
        val byDesc = d.wait(Until.hasObject(By.descContains(tag)), timeoutMs / 3)
        if (byDesc) return true
        return d.wait(Until.hasObject(By.textContains(tag)), timeoutMs / 3)
    }

    fun grantRuntimePermissionsRule(): TestRule {
        val permissions =
            mutableListOf(Manifest.permission.RECORD_AUDIO).apply {
                if (Build.VERSION.SDK_INT >= 33) {
                    add(Manifest.permission.POST_NOTIFICATIONS)
                }
            }
        return GrantPermissionRule.grant(*permissions.toTypedArray())
    }

    fun assertPackageForeground(packageName: String = ServiceProbe.packageName()) {
        val d = device()
        assertTrue(
            "expected package $packageName in foreground",
            d.currentPackageName == packageName ||
                d.wait(Until.hasObject(By.pkg(packageName).depth(0)), 5_000),
        )
    }

    fun pressHome() {
        device().pressHome()
        Thread.sleep(500)
    }

    fun sleepDevice() {
        // KEYCODE_SLEEP = 223
        device().executeShellCommand("input keyevent 223")
        Thread.sleep(1_000)
    }

    fun wakeDevice() {
        // KEYCODE_WAKEUP = 224
        device().executeShellCommand("input keyevent 224")
        device().pressMenu()
        device().wakeUp()
        Thread.sleep(500)
    }

    /**
     * Soft process death for the target app. Prefer `am kill` over force-stop:
     * force-stop also clears alarms/jobs and is a terminal admin action, not
     * representative of LMK/process death UX.
     */
    fun amKillTarget() {
        val pkg = ServiceProbe.packageName()
        device().executeShellCommand("am kill $pkg")
        Thread.sleep(1_500)
    }
}
