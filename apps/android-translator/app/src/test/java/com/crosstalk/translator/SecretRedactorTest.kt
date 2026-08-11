package com.crosstalk.translator

import com.crosstalk.translator.util.SecretRedactor
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SecretRedactorTest {
    @Test
    fun redactsBearerAndJwt() {
        val jwt =
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
                "eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature"
        val raw = "Authorization: Bearer $jwt token=$jwt"
        val redacted = SecretRedactor.redact(raw)
        assertFalse(redacted.contains(jwt))
        assertTrue(redacted.contains("***"))
    }
}
