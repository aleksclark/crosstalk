package com.crosstalk.translator.auth

/**
 * Encrypted refresh-token envelope stored in DataStore.
 * Plaintext access JWT is never written here.
 */
data class SessionCredential(
    val ciphertextBase64: String,
    val ivBase64: String,
    val formatVersion: Int = FORMAT_VERSION,
) {
    companion object {
        const val FORMAT_VERSION: Int = 1
    }
}
