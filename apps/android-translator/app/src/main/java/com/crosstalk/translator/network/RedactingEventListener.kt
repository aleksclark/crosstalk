package com.crosstalk.translator.network

import com.crosstalk.translator.util.SecretRedactor
import okhttp3.Call
import okhttp3.EventListener
import okhttp3.Protocol
import okhttp3.Response
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Proxy
import java.util.concurrent.atomic.AtomicBoolean

/**
 * OkHttp event listener that never records Authorization headers, query tokens,
 * or full URLs. Diagnostics stay path-template oriented.
 */
class RedactingEventListener(
    private val log: (String) -> Unit = {},
) : EventListener() {
    private val enabled = AtomicBoolean(true)

    override fun callStart(call: Call) {
        if (!enabled.get()) return
        log("http_call_start method=${call.request().method} host=${call.request().url.host}")
    }

    override fun connectStart(call: Call, inetSocketAddress: InetSocketAddress, proxy: Proxy) {
        if (!enabled.get()) return
        log("http_connect_start host=${inetSocketAddress.hostString}")
    }

    override fun connectEnd(
        call: Call,
        inetSocketAddress: InetSocketAddress,
        proxy: Proxy,
        protocol: Protocol?,
    ) {
        if (!enabled.get()) return
        log("http_connect_end host=${inetSocketAddress.hostString} protocol=${protocol ?: "unknown"}")
    }

    override fun responseHeadersEnd(call: Call, response: Response) {
        if (!enabled.get()) return
        log("http_response code=${response.code} host=${call.request().url.host}")
    }

    override fun callFailed(call: Call, ioe: IOException) {
        if (!enabled.get()) return
        val msg = SecretRedactor.redact(ioe.message)
        log("http_call_failed host=${call.request().url.host} error=$msg")
    }
}
