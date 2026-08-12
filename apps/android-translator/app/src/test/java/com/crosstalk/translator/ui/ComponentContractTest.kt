package com.crosstalk.translator.ui

import com.crosstalk.translator.feature.live.LiveSessionUiState
import com.crosstalk.translator.feature.live.LiveSessionViewModel
import com.crosstalk.translator.feature.live.MicPermissionUi
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.service.ServiceState
import com.crosstalk.translator.ui.theme.CtColors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Lightweight component/contract checks that do not require a Compose host.
 * Full screenshot matrix is documented under docs/screenshots/.
 */
class ComponentContractTest {
    @Test
    fun statusTonesDoNotReuseBrandAccentAsSemanticOk() {
        val dark = CtColors.dark()
        // Status OK must not be the brand cyan accent.
        assertNotEquals(dark.accent, dark.statusOk)
        assertNotEquals(dark.accent, dark.statusDanger)
        assertNotEquals(dark.accent, dark.statusWarning)
    }

    @Test
    fun onlyOneBrandAccentExists() {
        val dark = CtColors.dark().accent
        val light = CtColors.light().accent
        assertEquals(dark.red, light.red, 0.001f)
        assertEquals(dark.green, light.green, 0.001f)
        assertEquals(dark.blue, light.blue, 0.001f)
        assertEquals(0x3D / 255f, dark.red, 0.01f)
        assertEquals(0xE0 / 255f, dark.green, 0.01f)
        assertEquals(0xF0 / 255f, dark.blue, 0.01f)
        // Forbidden alternate accents must not appear as product accent.
        val forbidden = listOf(0xFFB98BFF, 0xFF7CFFB2)
        forbidden.forEach { hex ->
            assertNotEquals(hex, CtColors.AccentArgb)
        }
    }

    @Test
    fun micPermissionStatesDriveHumanStatus() {
        val base = LiveSessionUiState()
        assertTrue(
            LiveSessionViewModel.buildStatusSentence(
                base.copy(micPermission = MicPermissionUi.Denied),
                ServiceState.Idle,
            ).contains("Microphone", ignoreCase = true),
        )
        assertTrue(
            LiveSessionViewModel.buildStatusSentence(
                base.copy(micPermission = MicPermissionUi.PermanentlyDenied),
                ServiceState.Idle,
            ).contains("Settings", ignoreCase = true),
        )
        assertTrue(
            LiveSessionViewModel.buildStatusSentence(
                base.copy(micPermission = MicPermissionUi.RevokedLive),
                ServiceState(phase = ServicePhase.Connected),
            ).contains("revoked", ignoreCase = true),
        )
    }

    @Test
    fun failedPhaseUsesErrorReason() {
        val sentence = LiveSessionViewModel.buildStatusSentence(
            LiveSessionUiState(micPermission = MicPermissionUi.Granted),
            ServiceState(phase = ServicePhase.Failed, errorReason = "ICE failed"),
        )
        assertEquals("ICE failed", sentence)
    }

    @Test
    fun connectingPhasesAreNotIdle() {
        listOf(
            ServicePhase.Preparing,
            ServicePhase.Minting,
            ServicePhase.Signaling,
            ServicePhase.IceChecking,
            ServicePhase.Connected,
            ServicePhase.Muted,
            ServicePhase.ReconnectScheduled,
            ServicePhase.WaitingForNetwork,
        ).forEach { phase ->
            assertTrue(ServiceState(phase = phase).isLiveOrConnecting)
        }
        assertFalse(ServiceState(phase = ServicePhase.Idle).isLiveOrConnecting)
        assertFalse(ServiceState(phase = ServicePhase.Stopped).isLiveOrConnecting)
        assertFalse(ServiceState(phase = ServicePhase.ProcessRestored).isLiveOrConnecting)
    }
}
