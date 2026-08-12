package com.crosstalk.translator.service

/**
 * Idempotent commands accepted by [TranslatorAudioService].
 * UI / notification actions map into these; the service owns all media side effects.
 */
sealed class ServiceCommand {
    data class Join(
        val sessionId: String,
        val sessionName: String,
        val feedName: String,
        val broadcastName: String,
        val feedIds: List<String> = emptyList(),
        val broadcastIds: List<String> = emptyList(),
    ) : ServiceCommand() {
        init {
            require(sessionId.isNotBlank()) { "sessionId required" }
            require(sessionName.isNotBlank()) { "sessionName required" }
        }
    }

    data class SetMuted(val muted: Boolean) : ServiceCommand()

    data object Stop : ServiceCommand()
}
