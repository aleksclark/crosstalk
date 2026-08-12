package com.crosstalk.translator.network

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TlsPolicyTest {
    @Test
    fun releaseRejectsCleartext() {
        var failed = false
        try {
            TlsPolicy.assertSafeBaseUrl("http://crosstalk.example", allowCleartext = false)
        } catch (_: IllegalArgumentException) {
            failed = true
        }
        assertTrue(failed)
    }

    @Test
    fun releaseAcceptsHttps() {
        TlsPolicy.assertSafeBaseUrl("https://crosstalk.example", allowCleartext = false)
    }

    @Test
    fun debugMayAllowLocalhostCleartext() {
        TlsPolicy.assertSafeBaseUrl("http://10.0.2.2:8080", allowCleartext = true)
        TlsPolicy.assertSafeBaseUrl("http://localhost:8080", allowCleartext = true)
    }

    @Test
    fun rejectsEmbeddedCredentialsAndQuery() {
        var credsRejected = false
        try {
            TlsPolicy.assertSafeBaseUrl("https://user:pass@host", allowCleartext = false)
        } catch (_: IllegalArgumentException) {
            credsRejected = true
        }
        assertTrue(credsRejected)

        var queryRejected = false
        try {
            TlsPolicy.assertSafeBaseUrl("https://host/?token=x", allowCleartext = false)
        } catch (_: IllegalArgumentException) {
            queryRejected = true
        }
        assertTrue(queryRejected)
    }

    @Test
    fun loopbackDetection() {
        assertTrue(TlsPolicy.isLoopbackHost("localhost"))
        assertTrue(TlsPolicy.isLoopbackHost("127.0.0.1"))
        assertTrue(TlsPolicy.isLoopbackHost("10.0.2.2"))
        assertFalse(TlsPolicy.isLoopbackHost("crosstalk.example"))
    }
}
