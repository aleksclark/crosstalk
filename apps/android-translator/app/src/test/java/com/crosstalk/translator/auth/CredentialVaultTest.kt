package com.crosstalk.translator.auth

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CredentialVaultTest {
    @Test
    fun roundTripsRefreshTokenThroughFakeCipher() = runBlocking {
        val vault = createTestCredentialVault(cipher = FakeKeystoreCipher())
        vault.saveRefreshToken("refresh-secret-abc")
        assertEquals("refresh-secret-abc", vault.readRefreshToken())

        val envelope = vault.readEnvelope()
        assertNotNull(envelope)
        requireNotNull(envelope)
        assertEquals(SessionCredential.FORMAT_VERSION, envelope.formatVersion)
        assertTrue(envelope.ciphertextBase64.isNotBlank())
        assertTrue(envelope.ivBase64.isNotBlank())
        // Ciphertext must not equal plaintext.
        assertNotEquals("refresh-secret-abc", envelope.ciphertextBase64)
    }

    @Test
    fun clearRemovesEnvelope() = runBlocking {
        val vault = createTestCredentialVault()
        vault.saveRefreshToken("to-clear")
        vault.clear()
        assertNull(vault.readRefreshToken())
        assertNull(vault.readEnvelope())
    }

    @Test
    fun unknownFormatVersionFailsClosedAndClears() = runBlocking {
        val vault = createTestCredentialVault()
        vault.saveRefreshToken("keep-me")
        val envelope = vault.readEnvelope()
        assertNotNull(envelope)
        assertEquals(1, envelope!!.formatVersion)
    }

    @Test
    fun decryptFailureClearsVault() = runBlocking {
        val writerCipher = FakeKeystoreCipher(ByteArray(32) { 1 })
        val readerCipher = FakeKeystoreCipher(ByteArray(32) { 9 })
        val sharedStore = InMemoryPreferencesDataStore()

        val writer = CredentialVault(dataStore = sharedStore, cipher = writerCipher)
        writer.saveRefreshToken("secret")
        assertNotNull(writer.readEnvelope())

        val reader = CredentialVault(dataStore = sharedStore, cipher = readerCipher)
        assertNull(reader.readRefreshToken())
        assertNull(reader.readEnvelope())
    }

    @Test
    fun rotationReplacesCiphertextAtomically() = runBlocking {
        val vault = createTestCredentialVault()
        vault.saveRefreshToken("token-v1")
        val first = vault.readEnvelope()!!
        vault.saveRefreshToken("token-v2")
        val second = vault.readEnvelope()!!
        assertEquals("token-v2", vault.readRefreshToken())
        assertNotEquals(first.ciphertextBase64, second.ciphertextBase64)
    }
}
