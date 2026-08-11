package com.crosstalk.translator.util

/**
 * Instrumentation-time environment for optional real-server tests.
 *
 * Real Pion / assignment tests run only when [CROSSTALK_BASE_URL] is set and reachable.
 * Never log credentials. Passwords/tokens must come from instrumentation args or env
 * that the harness injects without echoing.
 */
object AndroidTestEnv {
    const val ARG_BASE_URL = "CROSSTALK_BASE_URL"
    const val ARG_ADMIN_USER = "CROSSTALK_ADMIN_USER"
    const val ARG_ADMIN_PASSWORD = "CROSSTALK_ADMIN_PASSWORD"
    const val ARG_TRANSLATOR_USER = "CROSSTALK_TRANSLATOR_USER"
    const val ARG_TRANSLATOR_PASSWORD = "CROSSTALK_TRANSLATOR_PASSWORD"
    const val ARG_SESSION_ID = "CROSSTALK_SESSION_ID"
    const val ARG_SESSION_NAME = "CROSSTALK_SESSION_NAME"

    /** Application id for debug builds (suffix .debug). */
    const val DEBUG_PACKAGE = "com.crosstalk.translator.debug"
    const val RELEASE_PACKAGE = "com.crosstalk.translator"

    fun instrumentationArg(key: String): String? {
        val bundle = androidx.test.platform.app.InstrumentationRegistry.getArguments()
        val fromArgs = bundle.getString(key)?.trim()?.takeIf { it.isNotEmpty() }
        if (fromArgs != null) return fromArgs
        return System.getenv(key)?.trim()?.takeIf { it.isNotEmpty() }
    }

    fun baseUrl(): String? = instrumentationArg(ARG_BASE_URL)?.trimEnd('/')

    fun realServerConfigured(): Boolean = !baseUrl().isNullOrBlank()

    fun adminUser(): String = instrumentationArg(ARG_ADMIN_USER) ?: "admin"

    fun adminPassword(): String? = instrumentationArg(ARG_ADMIN_PASSWORD)

    fun translatorUser(): String? = instrumentationArg(ARG_TRANSLATOR_USER)

    fun translatorPassword(): String? = instrumentationArg(ARG_TRANSLATOR_PASSWORD)

    fun sessionId(): String? = instrumentationArg(ARG_SESSION_ID)

    fun sessionName(): String? = instrumentationArg(ARG_SESSION_NAME)

    fun ignoreReasonNoServer(): String =
        "Requires $ARG_BASE_URL (and seed credentials) pointing at a live CrossTalk server. " +
            "Run via test/android/run-device-golden.sh or connectedDebugAndroidTest -Pandroid.testInstrumentationRunnerArguments.$ARG_BASE_URL=..."
}
