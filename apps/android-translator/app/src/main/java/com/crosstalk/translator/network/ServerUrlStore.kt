package com.crosstalk.translator.network

import android.content.Context
import android.content.SharedPreferences

/** Persists the non-secret server origin selected by the operator. */
class ServerUrlStore(
    private val preferences: SharedPreferences,
    private val defaultUrl: String,
) {
    constructor(context: Context, defaultUrl: String) : this(
        preferences = context.applicationContext.getSharedPreferences(
            PREFERENCES_NAME,
            Context.MODE_PRIVATE,
        ),
        defaultUrl = defaultUrl,
    )

    fun read(): String = preferences.getString(KEY_SERVER_URL, null) ?: defaultUrl

    fun save(serverUrl: String) {
        check(preferences.edit().putString(KEY_SERVER_URL, serverUrl).commit()) {
            "Unable to persist server URL"
        }
    }

    private companion object {
        const val PREFERENCES_NAME = "crosstalk_server"
        const val KEY_SERVER_URL = "server_url"
    }
}
