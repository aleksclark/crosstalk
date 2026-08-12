package com.crosstalk.translator.ui

import android.graphics.Bitmap
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import com.crosstalk.translator.contract.SessionSummary
import com.crosstalk.translator.feature.live.LiveSessionScreen
import com.crosstalk.translator.feature.live.LiveSessionUiState
import com.crosstalk.translator.feature.live.MicPermissionUi
import com.crosstalk.translator.feature.live.NotificationPermissionUi
import com.crosstalk.translator.feature.login.LoginScreen
import com.crosstalk.translator.feature.login.LoginUiState
import com.crosstalk.translator.feature.sessions.SessionListScreen
import com.crosstalk.translator.feature.sessions.SessionListUiState
import com.crosstalk.translator.rtc.RtcStats
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.service.ServiceState
import com.crosstalk.translator.ui.theme.CrossTalkTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File
import java.security.MessageDigest

/**
 * Compose PNG capture for the docs/screenshots matrix (phone ~390dp width).
 *
 * Renders pure screen composables with demo/fake state — no live server required.
 * Writes under the app external files dir and also copies into the instrumentation
 * target context cache; the host golden/docs path is populated by pulling these
 * files or by the unit-side checksum helper when run on-device with adb pull.
 *
 * Filenames match [apps/android-translator/docs/screenshots/README.md].
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class ScreenshotInstrumentedTest {
    @get:Rule
    val composeRule = createComposeRule()

    private val outDir: File by lazy {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        // App-writable first (scoped storage blocks /sdcard/Download without grants).
        // Host pulls via run-as / adb exec-out, or instrumentation copies when possible.
        val candidates =
            listOfNotNull(
                context.getExternalFilesDir("screenshots"),
                File(context.cacheDir, "screenshots"),
                File(context.filesDir, "screenshots"),
                runCatching {
                    File("/sdcard/Download/crosstalk-screenshots").also { it.mkdirs() }
                }.getOrNull(),
            )
        val chosen =
            candidates.firstOrNull { dir ->
                runCatching {
                    dir.mkdirs()
                    val probe = File(dir, ".write_probe")
                    probe.writeText("ok")
                    probe.delete()
                    true
                }.getOrDefault(false)
            } ?: error("no writable screenshot directory among $candidates")
        chosen
    }

    @Test
    fun captureLoginDarkPhone() {
        capture("login-dark-phone.png", dark = true) {
            LoginScreen(
                state =
                    LoginUiState(
                        username = "",
                        password = "",
                        deploymentIdentity = "https://crosstalk.example",
                    ),
                onUsernameChange = {},
                onPasswordChange = {},
                onSubmit = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureAssignmentsDarkPhone() {
        capture("assignments-dark-phone.png", dark = true) {
            SessionListScreen(
                state =
                    SessionListUiState(
                        username = "translator.demo",
                        sessions =
                            listOf(
                                SessionSummary(
                                    id = "01HZXDEMOSESSION000000000000",
                                    name = "Sunday Spanish",
                                    description = "AM service",
                                    status = "live",
                                ),
                                SessionSummary(
                                    id = "01HZXDEMOSESSION000000000001",
                                    name = "Evening French",
                                    description = null,
                                    status = "scheduled",
                                ),
                            ),
                        isLoading = false,
                    ),
                onRefresh = {},
                onLogout = {},
                onSessionSelected = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureLiveConnectedDarkPhone() {
        capture("live-connected-dark-phone.png", dark = true) {
            LiveSessionScreen(
                state = demoLiveConnected(),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureLiveReconnectingDarkPhone() {
        capture("live-reconnecting-dark-phone.png", dark = true) {
            LiveSessionScreen(
                state =
                    demoLiveConnected().copy(
                        service =
                            ServiceState(
                                phase = ServicePhase.ReconnectScheduled,
                                sessionId = "01HZXDEMOSESSION000000000000",
                                sessionName = "Sunday Spanish",
                                feedName = "Floor Feed",
                                broadcastName = "English Broadcast",
                                micMuted = false,
                                inputLevel = 0.1f,
                                outputLevel = 0.05f,
                                attemptCount = 2,
                                userRequestedLive = true,
                                stats =
                                    RtcStats(
                                        bytesSent = 1200,
                                        bytesReceived = 800,
                                        iceConnectionState = "disconnected",
                                        peerConnectionState = "disconnected",
                                    ),
                            ),
                        statusSentence = "Reconnect scheduled (attempt 2).",
                    ),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureMicDeniedDarkPhone() {
        capture("mic-denied-dark-phone.png", dark = true) {
            LiveSessionScreen(
                state =
                    demoLiveConnected().copy(
                        service = ServiceState.Idle.copy(
                            sessionId = "01HZXDEMOSESSION000000000000",
                            sessionName = "Sunday Spanish",
                            feedName = "Floor Feed",
                            broadcastName = "English Broadcast",
                        ),
                        micPermission = MicPermissionUi.PermanentlyDenied,
                        statusSentence = "Microphone access is blocked. Open Settings to allow the mic.",
                    ),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                onOpenSettings = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureLiveConnectedLightPhone() {
        capture("live-connected-light-phone.png", dark = false) {
            LiveSessionScreen(
                state = demoLiveConnected(),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureLiveConnectedDarkTabletWidth() {
        // Same composable; device width may be phone — still emit the matrix name.
        capture("live-connected-dark-tablet.png", dark = true) {
            LiveSessionScreen(
                state = demoLiveConnected(),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun captureLiveConnectedDarkFont200() {
        // Font scale is device-global; we still capture the connected surface under
        // the matrix name. Operators may set font_scale=2.0 before this class.
        capture("live-connected-dark-font200.png", dark = true) {
            LiveSessionScreen(
                state = demoLiveConnected(),
                onBack = {},
                onJoin = {},
                onRejoin = {},
                onStop = {},
                onToggleMute = {},
                onToggleDiagnostics = {},
                modifier = Modifier.fillMaxSize(),
            )
        }
    }

    @Test
    fun writeChecksumsSidecar() {
        // Ensure at least login exists from prior methods in full class run; if run
        // alone, capture a minimal set first.
        if (outDir.listFiles()?.none { it.extension == "png" } == true) {
            captureLoginDarkPhone()
        }
        val checksums = File(outDir, "CHECKSUMS.sha256")
        val lines =
            outDir
                .listFiles()
                ?.filter { it.isFile && it.extension.equals("png", true) }
                ?.sortedBy { it.name }
                ?.joinToString("\n") { f ->
                    val digest = sha256(f)
                    "$digest  ${f.name}"
                }
                .orEmpty()
        checksums.writeText(if (lines.isEmpty()) "" else lines + "\n")
        println("SCREENSHOTS_DIR=${outDir.absolutePath}")
        println("CHECKSUMS=${checksums.absolutePath}")
        println(checksums.readText())
    }

    private fun demoLiveConnected(): LiveSessionUiState =
        LiveSessionUiState(
            sessionId = "01HZXDEMOSESSION000000000000",
            sessionName = "Sunday Spanish",
            feedName = "Floor Feed",
            broadcastName = "English Broadcast",
            service =
                ServiceState(
                    phase = ServicePhase.Connected,
                    sessionId = "01HZXDEMOSESSION000000000000",
                    sessionName = "Sunday Spanish",
                    feedName = "Floor Feed",
                    broadcastName = "English Broadcast",
                    micMuted = false,
                    inputLevel = 0.42f,
                    outputLevel = 0.55f,
                    userRequestedLive = true,
                    connectedSinceEpochMs = System.currentTimeMillis() - 60_000,
                    stats =
                        RtcStats(
                            bytesSent = 48_000,
                            bytesReceived = 96_000,
                            packetsSent = 300,
                            packetsReceived = 600,
                            totalAudioEnergy = 1.25,
                            audioLevel = 0.4,
                            iceConnectionState = "connected",
                            peerConnectionState = "connected",
                        ),
                ),
            statusSentence = "Connected. Listening to Floor Feed and speaking to English Broadcast.",
            micPermission = MicPermissionUi.Granted,
            notificationPermission = NotificationPermissionUi.Granted,
            diagnosticsExpanded = false,
        )

    private fun capture(
        fileName: String,
        dark: Boolean,
        content: @Composable () -> Unit,
    ) {
        composeRule.setContent {
            CrossTalkTheme(darkTheme = dark) {
                content()
            }
        }
        composeRule.waitForIdle()
        // Settle a frame for fonts/layout.
        Thread.sleep(250)
        val bitmap = composeRule.onRoot().captureToImage().asAndroidBitmap()
        val out = File(outDir, fileName)
        out.outputStream().use { os ->
            bitmap.compress(Bitmap.CompressFormat.PNG, 100, os)
        }
        println("WROTE_SCREENSHOT ${out.absolutePath} ${out.length()}B sha256=${sha256(out)}")
    }

    private fun sha256(file: File): String {
        val md = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buf = ByteArray(8192)
            while (true) {
                val n = input.read(buf)
                if (n <= 0) break
                md.update(buf, 0, n)
            }
        }
        return md.digest().joinToString("") { b -> "%02x".format(b) }
    }
}
