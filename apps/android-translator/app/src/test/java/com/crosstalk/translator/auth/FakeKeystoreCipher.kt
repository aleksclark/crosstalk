package com.crosstalk.translator.auth

import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/** In-memory AES-GCM stand-in for JVM unit tests (no Android Keystore). Test source set only. */
class FakeKeystoreCipher(
    private val keyBytes: ByteArray = ByteArray(32) { (it + 7).toByte() },
) : KeystoreCipher {
    override fun encrypt(plaintext: ByteArray): EncryptedPayload {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val key = SecretKeySpec(keyBytes, "AES")
        cipher.init(Cipher.ENCRYPT_MODE, key)
        return EncryptedPayload(ciphertext = cipher.doFinal(plaintext), iv = cipher.iv)
    }

    override fun decrypt(payload: EncryptedPayload): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val key = SecretKeySpec(keyBytes, "AES")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, payload.iv))
        return cipher.doFinal(payload.ciphertext)
    }
}
