package com.crosstalk.translator.util

/**
 * Redacts secrets from log/diagnostic strings.
 * Phase 2 expands coverage and unit-tests canaries.
 */
object SecretRedactor {
    private val jwtLike =
        Regex("""\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b""")
    private val bearer =
        Regex("""(?i)(authorization\s*[:=]\s*bearer\s+)\S+""")
    private val tokenParam =
        Regex("""(?i)([?&](?:token|access_token|refresh_token)=)[^&\s]+""")
    private val labeledSecrets =
        Regex(
            """(?i)\b(access_token|refresh_token|token|password|authorization)\b\s*[:=]\s*\S+""",
        )

    fun redact(input: String?): String {
        if (input.isNullOrEmpty()) return input.orEmpty()
        var out = input
        out = bearer.replace(out, "$1***")
        out = tokenParam.replace(out, "$1***")
        out = labeledSecrets.replace(out) { m ->
            val label = m.groupValues.getOrNull(1) ?: "secret"
            "$label=***"
        }
        out = jwtLike.replace(out, "***JWT***")
        return out
    }
}
