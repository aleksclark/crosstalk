package com.crosstalk.translator.util

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * Minimal REST client for instrumented real-server tests.
 * Never logs tokens or passwords.
 */
class RealServerClient(
    private val baseUrl: String,
    private val client: OkHttpClient =
        OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .build(),
) {
    private val json = "application/json; charset=utf-8".toMediaType()

    fun login(username: String, password: String): Tokens {
        val body =
            JSONObject()
                .put("username", username)
                .put("password", password)
                .toString()
        val resp = post("/api/auth/login", body, bearer = null)
        return Tokens(
            accessToken = resp.getString("access_token"),
            refreshToken = resp.getString("refresh_token"),
        )
    }

    fun listSessions(accessToken: String): List<SessionRow> {
        val resp = get("/api/sessions", accessToken)
        val data = resp.optJSONArray("data") ?: JSONArray()
        return buildList {
            for (i in 0 until data.length()) {
                val o = data.getJSONObject(i)
                add(
                    SessionRow(
                        id = o.getString("id"),
                        name = o.getString("name"),
                        description = o.optString("description", ""),
                    ),
                )
            }
        }
    }

    fun listChannels(accessToken: String, sessionId: String): List<ChannelRow> {
        val resp = get("/api/sessions/$sessionId/channels", accessToken)
        val data = resp.optJSONArray("data") ?: JSONArray()
        return buildList {
            for (i in 0 until data.length()) {
                val o = data.getJSONObject(i)
                add(
                    ChannelRow(
                        id = o.getString("id"),
                        name = o.getString("name"),
                        type = o.getString("type"),
                    ),
                )
            }
        }
    }

    fun mintMediaTicket(accessToken: String, sessionId: String): MediaTicketRow {
        val body =
            JSONObject()
                .put("session_id", sessionId)
                .put("role", "translator")
                .toString()
        val resp = post("/api/webrtc/token", body, bearer = accessToken)
        return MediaTicketRow(
            token = resp.getString("token"),
            sessionId = resp.optString("session_id", sessionId),
            expiresAt = resp.optString("expires_at", ""),
            produceChannelIds = resp.optJSONArray("produce_channel_ids").toStringList(),
            listenChannelIds = resp.optJSONArray("listen_channel_ids").toStringList(),
        )
    }

    fun reachable(): Boolean =
        runCatching {
            val req =
                Request.Builder()
                    .url("$baseUrl/admin/")
                    .get()
                    .build()
            client.newCall(req).execute().use { it.code in 200..499 }
        }.getOrDefault(false)

    private fun get(path: String, bearer: String?): JSONObject {
        val builder = Request.Builder().url(baseUrl + path).get()
        if (bearer != null) builder.header("Authorization", "Bearer $bearer")
        client.newCall(builder.build()).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            check(resp.isSuccessful) { "GET $path -> ${resp.code}" }
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        }
    }

    private fun post(path: String, jsonBody: String, bearer: String?): JSONObject {
        val builder =
            Request.Builder()
                .url(baseUrl + path)
                .post(jsonBody.toRequestBody(json))
        if (bearer != null) builder.header("Authorization", "Bearer $bearer")
        client.newCall(builder.build()).execute().use { resp ->
            val text = resp.body?.string().orEmpty()
            check(resp.isSuccessful) { "POST $path -> ${resp.code}" }
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        }
    }

    private fun JSONArray?.toStringList(): List<String> {
        if (this == null) return emptyList()
        return buildList {
            for (i in 0 until length()) add(getString(i))
        }
    }

    data class Tokens(val accessToken: String, val refreshToken: String)
    data class SessionRow(val id: String, val name: String, val description: String)
    data class ChannelRow(val id: String, val name: String, val type: String)
    data class MediaTicketRow(
        val token: String,
        val sessionId: String,
        val expiresAt: String,
        val produceChannelIds: List<String>,
        val listenChannelIds: List<String>,
    )
}
