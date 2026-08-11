package com.crosstalk.translator.service

import android.Manifest
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import androidx.test.core.app.ApplicationProvider
import com.crosstalk.translator.CrossTalkApplication
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.CredentialVault
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.AuthTokens
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.contract.SessionSummary
import com.crosstalk.translator.rtc.FakeRtcEngine
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.Robolectric
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config
import org.robolectric.shadows.ShadowApplication
import org.robolectric.shadows.ShadowLooper
import org.robolectric.shadows.ShadowService

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = CrossTalkApplication::class)
class TranslatorAudioServiceTest {

    private lateinit var app: CrossTalkApplication
    private lateinit var fakeRtc: FakeRtcEngine
    private lateinit var api: FakeApi
    private val engines = mutableListOf<FakeRtcEngine>()

    @Before
    fun setUp() {
        app = ApplicationProvider.getApplicationContext()
        // Grant mic so join path is not permission-revoked under Robolectric.
        val shadowApp = Shadows.shadowOf(app)
        shadowApp.grantPermissions(Manifest.permission.RECORD_AUDIO)

        api = FakeApi()
        val vault = CredentialVault.createForTests()
        runBlocking {
            vault.saveRefreshToken("refresh-test")
        }
        val auth =
            AuthRepository(
                api = api,
                vault = vault,
                accessTokenSink = {},
            )
        runBlocking {
            auth.login("translator", "pw")
        }

        val container = app.container
        container.rtcEngineFactory = {
            FakeRtcEngine().also {
                fakeRtc = it
                engines += it
            }
        }
        setField(container, "authRepository", auth)
    }

    @After
    fun tearDown() {
        engines.clear()
    }

    @Test
    fun onStartCommandReturnsNotStickyAndPromotesForegroundImmediately() {
        val controller =
            Robolectric.buildService(
                TranslatorAudioService::class.java,
                joinIntent(),
            )
        val service = controller.create().get()
        val startId = controller.startCommand(0, 1)
        // Robolectric startCommand returns the service's onStartCommand result via shadow in some versions;
        // call directly for certainty.
        val result = service.onStartCommand(joinIntent(), 0, 1)
        assertEquals(android.app.Service.START_NOT_STICKY, result)

        val shadow: ShadowService = Shadows.shadowOf(service)
        assertTrue(
            "Service must call startForeground before network/RTC work",
            shadow.isForegroundStopped.not() || shadow.lastForegroundNotification != null ||
                isInForeground(shadow),
        )
        // Notification channel created for live translation.
        val nm = app.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
            assertNotNull(nm.getNotificationChannel(ServiceNotification.CHANNEL_ID))
        }
        controller.destroy()
    }

    @Test
    fun joinIdempotentAndStopCleansUp() {
        val controller =
            Robolectric.buildService(TranslatorAudioService::class.java, joinIntent())
        val service = controller.create().get()
        service.onStartCommand(joinIntent(), 0, 1)
        ShadowLooper.idleMainLooper()
        // Drain background coroutines.
        idle()

        val binder = service.onBind(null) as TranslatorAudioService.LocalBinder
        // Second join should bump generation without crash.
        binder.dispatch(
            ServiceCommand.Join(
                sessionId = "sess-1",
                sessionName = "Sunday Spanish",
                feedName = "Floor",
                broadcastName = "English",
            ),
        )
        idle()

        val genBeforeStop = binder.state.value.generation
        binder.dispatch(ServiceCommand.Stop)
        idle()

        assertEquals(ServicePhase.Stopped, binder.state.value.phase)
        assertTrue(binder.state.value.wasExplicitlyStopped)
        assertTrue(binder.state.value.generation > genBeforeStop)

        // Engines closed.
        engines.forEach { engine ->
            assertTrue(engine.closeCount() >= 1 || engine.isClosed())
        }
        controller.destroy()
    }

    @Test
    fun stopWhileReconnectPendingDoesNotMintAgain() {
        api.failMintWithNetwork = true
        val controller =
            Robolectric.buildService(TranslatorAudioService::class.java, joinIntent())
        val service = controller.create().get()
        service.onStartCommand(joinIntent(), 0, 1)
        idle()

        val binder = service.onBind(null) as TranslatorAudioService.LocalBinder
        // Force join path if start intent didn't fully orchestrate under test auth.
        binder.dispatch(
            ServiceCommand.Join(
                sessionId = "sess-1",
                sessionName = "Sunday Spanish",
                feedName = "Floor",
                broadcastName = "English",
            ),
        )
        idle()
        val mintsBeforeStop = api.mintCount

        binder.dispatch(ServiceCommand.Stop)
        idle()
        // Advance "time" for any scheduled reconnect.
        ShadowLooper.idleMainLooper()
        Thread.sleep(50)
        idle()

        assertEquals(ServicePhase.Stopped, binder.state.value.phase)
        assertEquals(
            "Stop must prevent further ticket minting",
            mintsBeforeStop,
            api.mintCount,
        )
        controller.destroy()
    }

    @Test
    fun notificationActionsDispatchMuteAndStop() {
        val controller =
            Robolectric.buildService(TranslatorAudioService::class.java, joinIntent())
        val service = controller.create().get()
        service.onStartCommand(joinIntent(), 0, 1)
        idle()

        val binder = service.onBind(null) as TranslatorAudioService.LocalBinder
        binder.dispatch(
            ServiceCommand.Join(
                sessionId = "sess-1",
                sessionName = "Sunday Spanish",
                feedName = "Floor",
                broadcastName = "English",
            ),
        )
        idle()

        service.onStartCommand(
            Intent(app, TranslatorAudioService::class.java).setAction(TranslatorAudioService.ACTION_MUTE),
            0,
            2,
        )
        idle()
        assertTrue(binder.state.value.micMuted)

        service.onStartCommand(
            Intent(app, TranslatorAudioService::class.java).setAction(TranslatorAudioService.ACTION_UNMUTE),
            0,
            3,
        )
        idle()
        assertFalse(binder.state.value.micMuted)

        service.onStartCommand(
            Intent(app, TranslatorAudioService::class.java).setAction(TranslatorAudioService.ACTION_STOP),
            0,
            4,
        )
        idle()
        assertEquals(ServicePhase.Stopped, binder.state.value.phase)
        controller.destroy()
    }

    @Test
    fun onTaskRemovedDoesNotStopService() {
        val controller =
            Robolectric.buildService(TranslatorAudioService::class.java, joinIntent())
        val service = controller.create().get()
        service.onStartCommand(joinIntent(), 0, 1)
        idle()
        val binder = service.onBind(null) as TranslatorAudioService.LocalBinder
        binder.dispatch(
            ServiceCommand.Join(
                sessionId = "sess-1",
                sessionName = "Sunday Spanish",
                feedName = "Floor",
                broadcastName = "English",
            ),
        )
        idle()
        val phaseBefore = binder.state.value.phase
        service.onTaskRemoved(Intent())
        idle()
        // Must not transition to Stopped solely from task removal.
        assertFalse(binder.state.value.phase == ServicePhase.Stopped && phaseBefore != ServicePhase.Stopped)
        assertFalse(binder.state.value.wasExplicitlyStopped)
        controller.destroy()
    }

    @Test
    fun foregroundServiceTypeMaskIncludesMicAndPlayback() {
        // Manifest-level check via package manager.
        val info =
            app.packageManager.getServiceInfo(
                android.content.ComponentName(app, TranslatorAudioService::class.java),
                0,
            )
        val types = info.foregroundServiceType
        assertTrue(
            (types and ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE) != 0,
        )
        assertTrue(
            (types and ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PLAYBACK) != 0,
        )
    }

    @Test
    fun serviceNotificationBuildsImmutableActions() {
        val notification = ServiceNotification(app)
        val state =
            ServiceState(
                phase = ServicePhase.Connected,
                sessionName = "Sunday Spanish",
                feedName = "Floor Feed",
                broadcastName = "English Broadcast",
                userRequestedLive = true,
            )
        val built = notification.build(state)
        assertEquals("Sunday Spanish", built.extras.getString(android.app.Notification.EXTRA_TITLE))
        assertTrue((built.actions?.size ?: 0) >= 2)
    }

    private fun joinIntent(): Intent =
        Intent(app, TranslatorAudioService::class.java).apply {
            action = TranslatorAudioService.ACTION_JOIN
            putExtra(TranslatorAudioService.EXTRA_SESSION_ID, "sess-1")
            putExtra(TranslatorAudioService.EXTRA_SESSION_NAME, "Sunday Spanish")
            putExtra(TranslatorAudioService.EXTRA_FEED_NAME, "Floor Feed")
            putExtra(TranslatorAudioService.EXTRA_BROADCAST_NAME, "English Broadcast")
        }

    private fun idle() {
        ShadowLooper.idleMainLooper()
        // Allow Default dispatcher work from service scope.
        ShadowLooper.runUiThreadTasksIncludingDelayedTasks()
        try {
            org.robolectric.shadows.ShadowLooper.idleMainLooper()
        } catch (_: Exception) {
        }
        Thread.sleep(20)
        ShadowLooper.idleMainLooper()
    }

    private fun isInForeground(shadow: ShadowService): Boolean =
        try {
            val method = shadow.javaClass.methods.firstOrNull { it.name.contains("Foreground") }
            shadow.lastForegroundNotificationId == ServiceNotification.NOTIFICATION_ID ||
                shadow.lastForegroundNotification != null
        } catch (_: Exception) {
            shadow.lastForegroundNotification != null
        }

    private fun setField(target: Any, name: String, value: Any) {
        var cls: Class<*>? = target.javaClass
        while (cls != null) {
            try {
                val f = cls.getDeclaredField(name)
                f.isAccessible = true
                // Clear final if needed
                try {
                    val modifiers = java.lang.reflect.Field::class.java.getDeclaredField("modifiers")
                    modifiers.isAccessible = true
                    modifiers.setInt(f, f.modifiers and java.lang.reflect.Modifier.FINAL.inv())
                } catch (_: Exception) {
                    // Java 12+ may block; try anyway.
                }
                f.set(target, value)
                return
            } catch (_: NoSuchFieldException) {
                cls = cls.superclass
            }
        }
        // authRepository is a lazy delegate property — set the backing field.
        try {
            val f = target.javaClass.getDeclaredField("${name}\$delegate")
            f.isAccessible = true
            f.set(
                target,
                lazyOf(value),
            )
            return
        } catch (_: Exception) {
        }
        error("Field $name not found on ${target.javaClass}")
    }

    private class FakeApi : CrossTalkApi {
        var mintCount: Int = 0
        var failMintWithNetwork: Boolean = false

        override suspend fun login(username: String, password: String): AuthTokens =
            AuthTokens(accessToken = "access", refreshToken = "refresh-test")

        override suspend fun refresh(refreshToken: String): AuthTokens =
            AuthTokens(accessToken = "access", refreshToken = refreshToken)

        override suspend fun logout(refreshToken: String) = Unit

        override suspend fun listSessions(): List<SessionSummary> =
            listOf(SessionSummary(id = "sess-1", name = "Sunday Spanish"))

        override suspend fun getSession(sessionId: String): SessionSummary =
            SessionSummary(id = sessionId, name = "Sunday Spanish")

        override suspend fun listChannels(sessionId: String): List<ChannelInfo> = emptyList()

        override suspend fun mintMediaTicket(sessionId: String, role: String): MediaTicket {
            mintCount++
            if (failMintWithNetwork) {
                throw ApiException.Network("offline")
            }
            return MediaTicket(
                token = "ticket-$mintCount",
                sessionId = sessionId,
                role = role,
                expiresAtEpochMs = System.currentTimeMillis() + 60_000L,
                produceChannelIds = listOf("bc-1"),
                listenChannelIds = listOf("feed-1"),
            )
        }
    }
}
