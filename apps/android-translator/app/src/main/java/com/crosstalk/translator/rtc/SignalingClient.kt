package com.crosstalk.translator.rtc

import com.crosstalk.translator.util.SecretRedactor
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong
import java.util.concurrent.atomic.AtomicReference

/**
 * Authenticated session signaling over WSS:
 * `.../api/sessions/{id}/ws?token=<url-encoded-ticket>`
 *
 * Never logs full URL, token, SDP, or ICE candidates — redacted diagnostics only.
 */
class SignalingClient(
    private val httpClient: OkHttpClient,
    private val log: (String) -> Unit = {},
) {
    enum class State {
        Idle,
        Connecting,
        Open,
        Closing,
        Closed,
        Failed,
    }

    sealed class Event {
        data class StateChanged(val state: State) : Event()

        data class Message(val message: SignalingMessage) : Event()

        data class Failed(val message: String) : Event()

        data class Closed(val code: Int, val reason: String) : Event()
    }

    private val eventsFlow =
        MutableSharedFlow<Event>(
            extraBufferCapacity = 64,
            onBufferOverflow = BufferOverflow.DROP_OLDEST,
        )
    val events: SharedFlow<Event> = eventsFlow.asSharedFlow()

    private val socket = AtomicReference<WebSocket?>(null)
    private val state = AtomicReference(State.Idle)
    private val generation = AtomicLong(0)
    private val closed = AtomicBoolean(false)
    private val outboundSeq = AtomicLong(0)

    fun state(): State = state.get()

    /**
     * Opens the signaling WebSocket. Idempotent close of any prior socket first.
     */
    fun connect(wsBaseUrl: String, sessionId: String, mediaTicket: String) {
        closeInternal(code = 1000, reason = "reconnect", emitClosed = false)
        closed.set(false)
        val gen = generation.incrementAndGet()
        val url = buildUrl(wsBaseUrl, sessionId, mediaTicket)
        setState(State.Connecting)
        log("signaling_connect ${redactUrl(url)}")

        val request = Request.Builder().url(url).build()
        val ws =
            httpClient.newWebSocket(
                request,
                object : WebSocketListener() {
                    override fun onOpen(webSocket: WebSocket, response: Response) {
                        if (!isCurrent(gen, webSocket)) return
                        setState(State.Open)
                        log("signaling_open host=${response.request.url.host} code=${response.code}")
                    }

                    override fun onMessage(webSocket: WebSocket, text: String) {
                        if (!isCurrent(gen, webSocket)) return
                        if (text.toByteArray(Charsets.UTF_8).size > SignalingCodec.MAX_MESSAGE_BYTES) {
                            log("signaling_reject oversized frame")
                            eventsFlow.tryEmit(Event.Failed("oversized signaling frame"))
                            return
                        }
                        try {
                            val msg = SignalingCodec.decode(text)
                            log("signaling_in ${SignalingCodec.diagnosticSummary(msg)}")
                            eventsFlow.tryEmit(Event.Message(msg))
                        } catch (e: SignalingCodecException) {
                            log("signaling_reject ${e.message}")
                            eventsFlow.tryEmit(Event.Failed(e.message ?: "malformed signaling"))
                        }
                    }

                    override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                        onMessage(webSocket, bytes.utf8())
                    }

                    override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                        if (!isCurrent(gen, webSocket)) return
                        setState(State.Closing)
                        log("signaling_closing code=$code reason=${SecretRedactor.redact(reason)}")
                        webSocket.close(code, reason)
                    }

                    override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                        if (!isCurrent(gen, webSocket)) return
                        socket.compareAndSet(webSocket, null)
                        setState(State.Closed)
                        log("signaling_closed code=$code reason=${SecretRedactor.redact(reason)}")
                        eventsFlow.tryEmit(Event.Closed(code, reason))
                    }

                    override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                        if (!isCurrent(gen, webSocket)) return
                        socket.compareAndSet(webSocket, null)
                        setState(State.Failed)
                        val msg = SecretRedactor.redact(t.message) ?: "signaling failure"
                        log("signaling_failed error=$msg")
                        eventsFlow.tryEmit(Event.Failed(msg))
                    }
                },
            )
        socket.set(ws)
    }

    /**
     * Sends a signaling message. Assigns a monotonic seq when absent.
     * @return false if socket is not open
     */
    fun send(message: SignalingMessage): Boolean {
        val ws = socket.get() ?: return false
        if (state.get() != State.Open) return false
        val withSeq =
            if (message.seq == null || message.seq == 0L) {
                message.copy(seq = outboundSeq.incrementAndGet())
            } else {
                message
            }
        val payload = SignalingCodec.encode(withSeq)
        if (payload.toByteArray(Charsets.UTF_8).size > SignalingCodec.MAX_MESSAGE_BYTES) {
            log("signaling_send_reject oversized")
            return false
        }
        log("signaling_out ${SignalingCodec.diagnosticSummary(withSeq)}")
        return ws.send(payload)
    }

    fun close(code: Int = 1000, reason: String = "client_close") {
        closeInternal(code, reason, emitClosed = true)
    }

    private fun closeInternal(code: Int, reason: String, emitClosed: Boolean) {
        closed.set(true)
        generation.incrementAndGet()
        val ws = socket.getAndSet(null) ?: run {
            if (state.get() != State.Idle && state.get() != State.Closed) {
                setState(State.Closed)
            }
            return
        }
        setState(State.Closing)
        try {
            ws.close(code, reason.take(120))
        } catch (_: Exception) {
            try {
                ws.cancel()
            } catch (_: Exception) {
                // ignore
            }
        }
        setState(State.Closed)
        if (emitClosed) {
            eventsFlow.tryEmit(Event.Closed(code, reason))
        }
    }

    private fun isCurrent(gen: Long, webSocket: WebSocket): Boolean {
        if (closed.get()) return false
        if (generation.get() != gen) return false
        return socket.get() === webSocket || state.get() == State.Connecting
    }

    private fun setState(next: State) {
        state.set(next)
        eventsFlow.tryEmit(Event.StateChanged(next))
    }

    companion object {
        fun buildUrl(wsBaseUrl: String, sessionId: String, mediaTicket: String): String {
            require(sessionId.isNotBlank()) { "sessionId required" }
            require(mediaTicket.isNotBlank()) { "mediaTicket required" }
            val trimmed = wsBaseUrl.trim().trimEnd('/')
            require(trimmed.isNotBlank()) { "wsBaseUrl required" }

            val wsRoot =
                when {
                    trimmed.startsWith("https://", ignoreCase = true) ->
                        "wss://" + trimmed.substring(8)
                    trimmed.startsWith("http://", ignoreCase = true) ->
                        "ws://" + trimmed.substring(7)
                    trimmed.startsWith("wss://", ignoreCase = true) ||
                        trimmed.startsWith("ws://", ignoreCase = true) -> trimmed
                    else -> throw IllegalArgumentException("wsBaseUrl must be http(s) or ws(s)")
                }

            val encodedTicket =
                URLEncoder.encode(mediaTicket, StandardCharsets.UTF_8.name())
            // sessionId is path-safe ULID in production; still encode path segment conservatively.
            val encodedSession =
                URLEncoder.encode(sessionId, StandardCharsets.UTF_8.name())
                    .replace("+", "%20")
            return "$wsRoot/api/sessions/$encodedSession/ws?token=$encodedTicket"
        }

        /** Redacts query token and any JWT-like material from a signaling URL. */
        fun redactUrl(url: String): String = SecretRedactor.redact(url)
    }
}
