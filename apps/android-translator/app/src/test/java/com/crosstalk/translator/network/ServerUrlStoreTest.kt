package com.crosstalk.translator.network

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34])
class ServerUrlStoreTest {
    @Test
    fun savedServerReplacesProductionDefaultAcrossStoreInstances() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val preferences = context.getSharedPreferences("server-url-test", Context.MODE_PRIVATE)
        preferences.edit().clear().commit()

        val first = ServerUrlStore(preferences, ServerEndpoint.PRODUCTION_BASE_URL)
        assertEquals(ServerEndpoint.PRODUCTION_BASE_URL, first.read())
        first.save("https://translation.example")

        val restored = ServerUrlStore(preferences, ServerEndpoint.PRODUCTION_BASE_URL)
        assertEquals("https://translation.example", restored.read())
    }
}
