package com.crosstalk.translator.util

/**
 * Redacts secrets from log/diagnostic strings.
 * Recognizes Authorization, token, access_token, refresh_token, JWT-like
 * strings, and WebSocket query parameters.
 */
object SecretRedactor {
    private const val REDACTED = "***"
    private const val JWT_PLACEHOLDER = "***JWT***"

    private val jwtLike =
        Regex("""\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b""")

    private val bearer =
        Regex("""(?i)(authorization\s*[:=]\s*bearer\s+)\S+""")

    private val authorizationHeader =
        Regex("""(?i)(authorization\s*[:=]\s*)\S+""")

    /** Query params on any URL, including wss://.../ws?token=... */
    private val tokenQueryParam =
        Regex("""(?i)([?&](?:token|access_token|refresh_token)=)[^&\s"'<>]+""")

    private val labeledSecrets =
        Regex(
            """(?i)\b(access_token|refresh_token|token|password|authorization)\b\s*[:=]\s*(?:Bearer\s+)?(\S+)""",
        )

    private val jsonSecretFields =
        Regex(
            """(?i)("(?:access_token|refresh_token|token|password|authorization)"\s*:\s*")([^"]*)(")""",
        )

    fun redact(input: String?): String {
        if (input.isNullOrEmpty()) return input.orEmpty()
        var out = input
        out = bearer.replace(out, "$1$REDACTED")
        out = authorizationHeader.replace(out, "$1$REDACTED")
        out = tokenQueryParam.replace(out, "$1$REDACTED")
        out = jsonSecretFields.replace(out, "$1$REDACTED$3")
        out = labeledSecrets.replace(out) { match ->
            val label = match.groupValues.getOrNull(1) ?: "secret"
            "$label=$REDACTED"
        }
        out = jwtLike.replace(out, JWT_PLACEHOLDER)
        return out
    }
}
