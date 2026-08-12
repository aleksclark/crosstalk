package com.crosstalk.translator.auth

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * Real Android Keystore AES-GCM round-trip for CredentialVault.
 * No FakeKeystoreCipher — this is the instrumented boundary.
 */
@RunWith(AndroidJUnit4::class)
class AuthKeystoreInstrumentedTest {
    @Test
    fun keystoreCipher_roundTrip_andUniqueIv() {
        val alias = "crosstalk_it_keystore_" + UUID.randomUUID().toString().take(8)
        val cipher = AndroidKeystoreCipher(keyAlias = alias)
        val plain = "refresh-token-canary-${UUID.randomUUID()}".toByteArray(Charsets.UTF_8)

        val a = cipher.encrypt(plain)
        val b = cipher.encrypt(plain)
        assertNotEquals(
            "GCM must randomize IV per encrypt",
            a.iv.toList(),
            b.iv.toList(),
        )
        assertNotEquals(
            "ciphertext must differ across encrypts of same plaintext",
            a.ciphertext.toList(),
            b.ciphertext.toList(),
        )

        val decoded = cipher.decrypt(a)
        assertTrue(plain.contentEquals(decoded))
    }

    @Test
    fun credentialVault_persistsEncryptedRefresh_only() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val fileName = "it_auth_${UUID.randomUUID()}.preferences_pb"
        val vault =
            CredentialVault.create(
                context = context,
                cipher = AndroidKeystoreCipher(keyAlias = "crosstalk_it_vault_" + UUID.randomUUID()),
                fileName = fileName,
            )
        val token = "rt_" + UUID.randomUUID().toString().replace("-", "")

        vault.saveRefreshToken(token)
        assertEquals(token, vault.readRefreshToken())

        val envelope = vault.readEnvelope()
        requireNotNull(envelope)
        assertTrue(envelope.ciphertextBase64.isNotBlank())
        assertTrue(envelope.ivBase64.isNotBlank())
        // Ciphertext must not equal plaintext token bytes as UTF-8 string.
        assertTrue(!envelope.ciphertextBase64.contains(token))

        vault.clear()
        assertNull(vault.readRefreshToken())
        assertNull(vault.readEnvelope())
    }
}
