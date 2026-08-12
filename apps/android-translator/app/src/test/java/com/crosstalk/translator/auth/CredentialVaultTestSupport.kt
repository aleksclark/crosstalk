package com.crosstalk.translator.auth

import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.emptyPreferences
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File

/** Test-only vault factory (no Android Main dispatcher / Keystore). */
fun createTestCredentialVault(
    cipher: KeystoreCipher = FakeKeystoreCipher(),
    initial: Preferences = emptyPreferences(),
): CredentialVault {
    return CredentialVault(
        dataStore = InMemoryPreferencesDataStore(initial),
        cipher = cipher,
    )
}

/** Back-compat alias used by existing unit tests. */
fun CredentialVault.Companion.createForTests(
    cipher: KeystoreCipher = FakeKeystoreCipher(),
    initial: Preferences = emptyPreferences(),
): CredentialVault = createTestCredentialVault(cipher, initial)

fun CredentialVault.Companion.createForTests(
    file: File,
    cipher: KeystoreCipher = FakeKeystoreCipher(),
): CredentialVault = createTestCredentialVault(cipher)

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
