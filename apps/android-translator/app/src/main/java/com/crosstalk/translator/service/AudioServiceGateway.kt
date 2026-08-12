package com.crosstalk.translator.service

import kotlinx.coroutines.flow.StateFlow

/**
 * Thin gateway used by ViewModels so they never hold a Service binder directly.
 */
interface AudioServiceGateway {
    val state: StateFlow<ServiceState>

    fun join(
        sessionId: String,
        sessionName: String,
        feedName: String,
        broadcastName: String,
        feedIds: List<String> = emptyList(),
        broadcastIds: List<String> = emptyList(),
    )

    fun setMuted(muted: Boolean)

    fun stop()

    fun bind()

    fun unbind()
}
