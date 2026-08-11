package com.crosstalk.translator.auth

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.emptyPreferences
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStoreFile
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File

/**
 * Persists only encrypted refresh-token envelopes in DataStore.
 * Access JWTs stay memory-only in [AuthRepository].
 */
class CredentialVault(
    private val dataStore: DataStore<Preferences>,
    private val cipher: KeystoreCipher,
) {
    private val mutex = Mutex()

    suspend fun saveRefreshToken(refreshToken: String) {
        require(refreshToken.isNotBlank()) { "refresh token must not be blank" }
        mutex.withLock {
            val encrypted = cipher.encrypt(refreshToken.toByteArray(Charsets.UTF_8))
            dataStore.edit { prefs ->
                prefs[KEY_CIPHERTEXT] = encrypted.ciphertext.toBase64()
                prefs[KEY_IV] = encrypted.iv.toBase64()
                prefs[KEY_FORMAT_VERSION] = SessionCredential.FORMAT_VERSION
            }
        }
    }

    suspend fun readRefreshToken(): String? = mutex.withLock {
        val prefs = dataStore.data.first()
        val ciphertext = prefs[KEY_CIPHERTEXT] ?: return@withLock null
        val iv = prefs[KEY_IV] ?: return@withLock null
        val version = prefs[KEY_FORMAT_VERSION] ?: return@withLock null
        if (version != SessionCredential.FORMAT_VERSION) {
            // Unknown envelope: fail closed and clear.
            clearLocked()
            return@withLock null
        }
        return@withLock try {
            val plain = cipher.decrypt(
                EncryptedPayload(
                    ciphertext = ciphertext.fromBase64(),
                    iv = iv.fromBase64(),
                ),
            )
            String(plain, Charsets.UTF_8).takeIf { it.isNotBlank() }
        } catch (_: Exception) {
            clearLocked()
            null
        }
    }

    suspend fun readEnvelope(): SessionCredential? = mutex.withLock {
        val prefs = dataStore.data.first()
        val ciphertext = prefs[KEY_CIPHERTEXT] ?: return@withLock null
        val iv = prefs[KEY_IV] ?: return@withLock null
        val version = prefs[KEY_FORMAT_VERSION] ?: return@withLock null
        SessionCredential(
            ciphertextBase64 = ciphertext,
            ivBase64 = iv,
            formatVersion = version,
        )
    }

    suspend fun clear() {
        mutex.withLock { clearLocked() }
    }

    private suspend fun clearLocked() {
        dataStore.edit { prefs ->
            prefs.remove(KEY_CIPHERTEXT)
            prefs.remove(KEY_IV)
            prefs.remove(KEY_FORMAT_VERSION)
        }
    }

    companion object {
        private val KEY_CIPHERTEXT = stringPreferencesKey("refresh_ciphertext")
        private val KEY_IV = stringPreferencesKey("refresh_iv")
        private val KEY_FORMAT_VERSION = intPreferencesKey("refresh_format_version")

        const val DATASTORE_FILE_NAME: String = "auth_credentials.preferences_pb"

        fun create(
            context: Context,
            cipher: KeystoreCipher = AndroidKeystoreCipher(),
            fileName: String = DATASTORE_FILE_NAME,
        ): CredentialVault {
            val appContext = context.applicationContext
            val store = PreferenceDataStoreFactory.create(
                produceFile = {
                    appContext.preferencesDataStoreFile(fileName.removeSuffix(".preferences_pb"))
                },
            )
            return CredentialVault(dataStore = store, cipher = cipher)
        }

        /**
         * JVM unit-test vault backed by an in-memory DataStore (no Android Main dispatcher).
         */
        fun createForTests(
            cipher: KeystoreCipher = FakeKeystoreCipher(),
            initial: Preferences = emptyPreferences(),
        ): CredentialVault {
            return CredentialVault(
                dataStore = InMemoryPreferencesDataStore(initial),
                cipher = cipher,
            )
        }

        /** @deprecated Prefer [createForTests] without a shared file. */
        fun createForTests(
            file: File,
            cipher: KeystoreCipher = FakeKeystoreCipher(),
        ): CredentialVault {
            // Keep signature for callers; ignore file to avoid multi-instance DataStore races.
            return createForTests(cipher = cipher)
        }
    }
}

/**
 * Minimal in-memory [DataStore] for unit tests. Avoids Android Main / file locks.
 */
internal class InMemoryPreferencesDataStore(
    initial: Preferences = emptyPreferences(),
) : DataStore<Preferences> {
    private val state = MutableStateFlow(initial)
    private val mutex = Mutex()

    override val data: Flow<Preferences> = state

    override suspend fun updateData(transform: suspend (t: Preferences) -> Preferences): Preferences =
        mutex.withLock {
            val next = transform(state.value)
            state.value = next
            next
        }
}
