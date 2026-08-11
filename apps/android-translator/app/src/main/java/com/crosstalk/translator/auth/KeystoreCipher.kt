package com.crosstalk.translator.auth

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * AES-256-GCM via Android Keystore. Ciphertext envelope is {ciphertext, iv};
 * the key never leaves the Keystore and is non-exportable.
 */
interface KeystoreCipher {
    fun encrypt(plaintext: ByteArray): EncryptedPayload
    fun decrypt(payload: EncryptedPayload): ByteArray
}

data class EncryptedPayload(
    val ciphertext: ByteArray,
    val iv: ByteArray,
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is EncryptedPayload) return false
        return ciphertext.contentEquals(other.ciphertext) && iv.contentEquals(other.iv)
    }

    override fun hashCode(): Int =
        31 * ciphertext.contentHashCode() + iv.contentHashCode()
}

class AndroidKeystoreCipher(
    private val keyAlias: String = DEFAULT_KEY_ALIAS,
) : KeystoreCipher {
    override fun encrypt(plaintext: ByteArray): EncryptedPayload {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, getOrCreateKey())
        val iv = cipher.iv
        val ciphertext = cipher.doFinal(plaintext)
        return EncryptedPayload(ciphertext = ciphertext, iv = iv)
    }

    override fun decrypt(payload: EncryptedPayload): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        val spec = GCMParameterSpec(GCM_TAG_BITS, payload.iv)
        cipher.init(Cipher.DECRYPT_MODE, getOrCreateKey(), spec)
        return cipher.doFinal(payload.ciphertext)
    }

    private fun getOrCreateKey(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        val existing = keyStore.getKey(keyAlias, null) as? SecretKey
        if (existing != null) return existing

        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        val spec = KeyGenParameterSpec.Builder(
            keyAlias,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(KEY_SIZE_BITS)
            .setUserAuthenticationRequired(false)
            .setRandomizedEncryptionRequired(true)
            .build()
        generator.init(spec)
        return generator.generateKey()
    }

    companion object {
        const val DEFAULT_KEY_ALIAS: String = "crosstalk_translator_refresh_v1"
        private const val ANDROID_KEYSTORE: String = "AndroidKeyStore"
        private const val TRANSFORMATION: String = "AES/GCM/NoPadding"
        private const val GCM_TAG_BITS: Int = 128
        private const val KEY_SIZE_BITS: Int = 256
    }
}

fun ByteArray.toBase64(): String =
    Base64.getEncoder().encodeToString(this)

fun String.fromBase64(): ByteArray =
    Base64.getDecoder().decode(this)
