package com.crosstalk.translator.app

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.crosstalk.translator.auth.createTestCredentialVault
import com.crosstalk.translator.network.ServerEndpoint
import com.crosstalk.translator.network.ServerUrlStore
import com.crosstalk.translator.service.AudioServiceGateway
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.service.ServiceState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class AppContainerTest {
    @Test
    fun changingServerClearsRefreshCredentialAndPersistsNewOrigin() = runTest {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val preferences = context.getSharedPreferences("app-container-server-test", Context.MODE_PRIVATE)
        preferences.edit().clear().commit()
        val serverStore = ServerUrlStore(preferences, ServerEndpoint.PRODUCTION_BASE_URL)
        val vault = createTestCredentialVault()
        vault.saveRefreshToken("production-refresh-token")
        val container = AppContainer(
            context = context,
            credentialVaultOverride = vault,
            serverUrlStoreOverride = serverStore,
        )

        container.configureServer("translation.example")

        assertEquals("https://translation.example", container.apiBaseUrl)
        assertEquals("https://translation.example", serverStore.read())
        assertNull(vault.readRefreshToken())
    }

    @Test
    fun changingServerIsRejectedWhileTranslationIsLive() = runTest {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val preferences = context.getSharedPreferences("app-container-live-server-test", Context.MODE_PRIVATE)
        preferences.edit().clear().commit()
        val serverStore = ServerUrlStore(preferences, ServerEndpoint.PRODUCTION_BASE_URL)
        val vault = createTestCredentialVault()
        vault.saveRefreshToken("production-refresh-token")
        val gateway = FakeAudioServiceGateway(
            ServiceState(
                phase = ServicePhase.Connected,
                userRequestedLive = true,
            ),
        )
        val container = AppContainer(
            context = context,
            credentialVaultOverride = vault,
            serverUrlStoreOverride = serverStore,
            audioServiceGatewayOverride = gateway,
        )

        val error = runCatching {
            container.configureServer("translation.example")
        }.exceptionOrNull()

        assertTrue(error is IllegalArgumentException)
        assertEquals(ServerEndpoint.PRODUCTION_BASE_URL, container.apiBaseUrl)
        assertEquals("production-refresh-token", vault.readRefreshToken())
    }

    private class FakeAudioServiceGateway(initialState: ServiceState) : AudioServiceGateway {
        override val state: StateFlow<ServiceState> = MutableStateFlow(initialState)

        override fun join(
            sessionId: String,
            sessionName: String,
            feedName: String,
            broadcastName: String,
            feedIds: List<String>,
            broadcastIds: List<String>,
        ) = Unit

        override fun setMuted(muted: Boolean) = Unit
        override fun stop() = Unit
        override fun bind() = Unit
        override fun unbind() = Unit
    }
}
