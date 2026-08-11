package com.crosstalk.translator

import com.crosstalk.translator.util.SecretRedactor
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SecretRedactorTest {
    private val jwt =
        "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
            "eyJzdWIiOiIxMjM0NTY3ODkwIn0." +
            "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

    @Test
    fun redactsAuthorizationBearer() {
        val raw = "Authorization: Bearer $jwt"
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains(jwt))
        assertTrue(redacted.contains("***"))
        assertFalse(redacted.contains("SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"))
    }

    @Test
    fun redactsAccessAndRefreshTokenLabels() {
        val raw = "access_token=$jwt refresh_token=opaque-refresh-xyz token=abc123"
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains(jwt))
        assertFalse(redacted.contains("opaque-refresh-xyz"))
        assertFalse(redacted.contains("abc123"))
        assertTrue(redacted.contains("access_token="))
        assertTrue(redacted.contains("refresh_token="))
    }

    @Test
    fun redactsJwtLikeStrings() {
        val raw = "user presented $jwt in logs"
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains(jwt))
        assertTrue(redacted.contains("***JWT***") || redacted.contains("***"))
    }

    @Test
    fun redactsWebSocketQueryToken() {
        val raw = "wss://crosstalk.example/api/sessions/01ABC/ws?token=media-ticket-secret-value"
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains("media-ticket-secret-value"))
        assertTrue(redacted.contains("token="))
        assertTrue(redacted.contains("***"))
    }

    @Test
    fun redactsJsonSecretFields() {
        val raw = """{"access_token":"$jwt","refresh_token":"r-secret","token":"t-secret"}"""
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains(jwt))
        assertFalse(redacted.contains("r-secret"))
        assertFalse(redacted.contains("t-secret"))
    }

    @Test
    fun leavesNonSecretTextIntact() {
        val raw = "http_response code=200 host=crosstalk.local path=/api/sessions"
        val redacted = SecretRedactor.redact(raw)
        assertTrue(redacted.contains("code=200"))
        assertTrue(redacted.contains("/api/sessions"))
    }
}
