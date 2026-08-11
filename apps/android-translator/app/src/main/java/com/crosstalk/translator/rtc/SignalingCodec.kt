package com.crosstalk.translator.rtc

import com.crosstalk.translator.util.SecretRedactor
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

/**
 * Encode/decode CrossTalk WebSocket signaling frames.
 *
 * Wire schema (server `SignalMessage`):
 * - type: "offer" | "answer" | "candidate" | "ice"
 * - sdp: string (offer/answer)
 * - candidate: { candidate, sdpMid, sdpMLineIndex, usernameFragment? }
 * - seq: optional monotonic correlation id
 *
 * Accepts both "candidate" and "ice" as inbound candidate types.
 * Never includes raw SDP/candidates in [diagnosticSummary].
 *
 * Uses kotlinx.serialization (not org.json) so JVM unit tests run without Android mocks.
 */
object SignalingCodec {
    const val TYPE_OFFER = "offer"
    const val TYPE_ANSWER = "answer"
    const val TYPE_CANDIDATE = "candidate"
    const val TYPE_ICE = "ice"

    /** Matches server default MaxSDPBytes + 4 KiB envelope headroom. */
    const val MAX_MESSAGE_BYTES: Int = (64 * 1024) + (4 * 1024)

    private val json =
        Json {
            ignoreUnknownKeys = true
            isLenient = false
            encodeDefaults = false
        }

    fun encode(message: SignalingMessage): String {
        val obj =
            buildJsonObject {
                put("type", message.type)
                message.sdp?.let { put("sdp", it) }
                message.candidate?.let { cand ->
                    put(
                        "candidate",
                        buildJsonObject {
                            cand.candidate?.let { put("candidate", it) }
                            cand.sdpMid?.let { put("sdpMid", it) }
                            cand.sdpMLineIndex?.let { put("sdpMLineIndex", it) }
                            cand.usernameFragment?.let { put("usernameFragment", it) }
                        },
                    )
                }
                if (message.seq != null && message.seq != 0L) {
                    put("seq", message.seq)
                }
            }
        return json.encodeToString(JsonObject.serializer(), obj)
    }

    /**
     * @throws SignalingCodecException on malformed JSON, unknown type, missing fields,
     * or oversized payload.
     */
    fun decode(raw: String): SignalingMessage {
        if (raw.toByteArray(Charsets.UTF_8).size > MAX_MESSAGE_BYTES) {
            throw SignalingCodecException("signaling message exceeds $MAX_MESSAGE_BYTES bytes")
        }
        val element =
            try {
                json.parseToJsonElement(raw)
            } catch (e: Exception) {
                throw SignalingCodecException("invalid JSON: ${e.message}")
            }
        val obj =
            try {
                element.jsonObject
            } catch (e: Exception) {
                throw SignalingCodecException("invalid JSON object: ${e.message}")
            }

        val type =
            obj.string("type")?.takeIf { it.isNotBlank() }
                ?: throw SignalingCodecException("missing type")

        val seq = obj.longOrNull("seq")

        return when (type) {
            TYPE_OFFER, TYPE_ANSWER -> {
                val sdp = obj.string("sdp")
                if (sdp.isNullOrBlank()) {
                    throw SignalingCodecException("$type missing sdp")
                }
                SignalingMessage(type = type, sdp = sdp, seq = seq)
            }
            TYPE_CANDIDATE, TYPE_ICE -> {
                val candEl = obj["candidate"]
                val init =
                    when {
                        candEl == null || candEl is JsonNull ->
                            throw SignalingCodecException("candidate message missing candidate")
                        candEl is JsonPrimitive && candEl.isString ->
                            IceCandidateInit(candidate = candEl.content)
                        candEl is JsonObject -> parseCandidate(candEl)
                        else -> throw SignalingCodecException("candidate has unexpected type")
                    }
                // Normalize wire type to "candidate" for consumers; "ice" is an alias.
                SignalingMessage(type = TYPE_CANDIDATE, candidate = init, seq = seq)
            }
            else -> throw SignalingCodecException("unknown signaling type: $type")
        }
    }

    fun diagnosticSummary(message: SignalingMessage): String {
        val parts = mutableListOf("type=${message.type}")
        message.seq?.let { parts += "seq=$it" }
        when (message.type) {
            TYPE_OFFER, TYPE_ANSWER -> {
                val len = message.sdp?.length ?: 0
                parts += "sdpBytes=$len"
            }
            TYPE_CANDIDATE, TYPE_ICE -> {
                val rawCand = message.candidate?.candidate.orEmpty()
                parts += "candidatePresent=${rawCand.isNotEmpty()}"
                parts += "sdpMid=${message.candidate?.sdpMid ?: "-"}"
                parts += "mLine=${message.candidate?.sdpMLineIndex ?: -1}"
                // Candidate type token is usually field 8 (0-based index 7) in a=candidate lines.
                val typ = rawCand.split(" ").getOrNull(7)
                if (typ != null) parts += "candType=$typ"
            }
        }
        return SecretRedactor.redact(parts.joinToString(" "))
    }

    fun diagnosticRaw(raw: String): String {
        val bytes = raw.toByteArray(Charsets.UTF_8).size
        return try {
            val msg = decode(raw)
            "bytes=$bytes ${diagnosticSummary(msg)}"
        } catch (e: SignalingCodecException) {
            "bytes=$bytes decode_error=${e.message}"
        }
    }

    private fun parseCandidate(obj: JsonObject): IceCandidateInit {
        return IceCandidateInit(
            candidate = obj.string("candidate"),
            sdpMid = obj.string("sdpMid"),
            sdpMLineIndex = obj.intOrNull("sdpMLineIndex"),
            usernameFragment = obj.string("usernameFragment"),
        )
    }

    private fun JsonObject.string(key: String): String? {
        val el = this[key] ?: return null
        if (el is JsonNull) return null
        return try {
            el.jsonPrimitive.contentOrNull?.takeIf { it.isNotEmpty() }
        } catch (_: Exception) {
            null
        }
    }

    private fun JsonObject.longOrNull(key: String): Long? {
        val el = this[key] ?: return null
        if (el is JsonNull) return null
        return try {
            el.jsonPrimitive.longOrNull
        } catch (_: Exception) {
            null
        }
    }

    private fun JsonObject.intOrNull(key: String): Int? {
        val el = this[key] ?: return null
        if (el is JsonNull) return null
        return try {
            el.jsonPrimitive.intOrNull
        } catch (_: Exception) {
            null
        }
    }
}

class SignalingCodecException(message: String) : Exception(message)
