package com.crosstalk.translator.service

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.crosstalk.translator.contract.LastSession
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.lastSessionDataStore: DataStore<Preferences> by preferencesDataStore(
    name = "crosstalk_last_session",
)

/**
 * Non-secret last-session identity for process-death UX. Never stores tickets/tokens.
 */
class LastSessionStore(
    context: Context,
) {
    private val dataStore = context.applicationContext.lastSessionDataStore

    suspend fun read(): LastSession? {
        val prefs = dataStore.data.first()
        val id = prefs[KEY_SESSION_ID] ?: return null
        val name = prefs[KEY_SESSION_NAME] ?: return null
        return LastSession(
            sessionId = id,
            sessionName = name,
            feedChannelName = prefs[KEY_FEED_NAME],
            broadcastChannelName = prefs[KEY_BROADCAST_NAME],
            wasExplicitlyStopped = prefs[KEY_EXPLICIT_STOP] ?: false,
        )
    }

    suspend fun save(session: LastSession) {
        dataStore.edit { prefs ->
            prefs[KEY_SESSION_ID] = session.sessionId
            prefs[KEY_SESSION_NAME] = session.sessionName
            if (session.feedChannelName != null) {
                prefs[KEY_FEED_NAME] = session.feedChannelName
            } else {
                prefs.remove(KEY_FEED_NAME)
            }
            if (session.broadcastChannelName != null) {
                prefs[KEY_BROADCAST_NAME] = session.broadcastChannelName
            } else {
                prefs.remove(KEY_BROADCAST_NAME)
            }
            prefs[KEY_EXPLICIT_STOP] = session.wasExplicitlyStopped
        }
    }

    suspend fun markExplicitlyStopped() {
        dataStore.edit { prefs ->
            if (prefs.contains(KEY_SESSION_ID)) {
                prefs[KEY_EXPLICIT_STOP] = true
            }
        }
    }

    suspend fun clear() {
        dataStore.edit { it.clear() }
    }

    companion object {
        private val KEY_SESSION_ID = stringPreferencesKey("session_id")
        private val KEY_SESSION_NAME = stringPreferencesKey("session_name")
        private val KEY_FEED_NAME = stringPreferencesKey("feed_name")
        private val KEY_BROADCAST_NAME = stringPreferencesKey("broadcast_name")
        private val KEY_EXPLICIT_STOP = booleanPreferencesKey("was_explicitly_stopped")
    }
}
