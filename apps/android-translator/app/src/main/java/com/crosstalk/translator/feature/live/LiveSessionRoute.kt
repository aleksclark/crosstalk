package com.crosstalk.translator.feature.live

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.crosstalk.translator.service.ServicePhase
import com.crosstalk.translator.ui.components.CtButton
import com.crosstalk.translator.ui.components.CtButtonVariant
import com.crosstalk.translator.ui.components.CtMetadata
import com.crosstalk.translator.ui.components.CtMetadataBlock
import com.crosstalk.translator.ui.components.CtMeter
import com.crosstalk.translator.ui.components.CtQrCode
import com.crosstalk.translator.ui.components.CtRule
import com.crosstalk.translator.ui.components.CtStatus
import com.crosstalk.translator.ui.components.CtStatusTone
import com.crosstalk.translator.ui.theme.CtTheme

@Composable
fun LiveSessionRoute(
    viewModel: LiveSessionViewModel,
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current

    val micLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        viewModel.onMicPermission(
            if (granted) MicPermissionUi.Granted else MicPermissionUi.Denied,
        )
    }
    val notificationLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { granted ->
        viewModel.onNotificationPermission(
            if (granted) NotificationPermissionUi.Granted else NotificationPermissionUi.Denied,
        )
    }

    fun refreshMicPermission() {
        val granted = ContextCompat.checkSelfPermission(
            context,
            Manifest.permission.RECORD_AUDIO,
        ) == PackageManager.PERMISSION_GRANTED
        val current = state.micPermission
        when {
            granted && current == MicPermissionUi.RevokedLive ->
                viewModel.onMicPermission(MicPermissionUi.Granted)
            granted -> viewModel.onMicPermission(MicPermissionUi.Granted)
            current == MicPermissionUi.Granted && state.service.isLiveOrConnecting ->
                viewModel.onMicPermission(MicPermissionUi.RevokedLive)
            current == MicPermissionUi.NotRequested ||
                current == MicPermissionUi.Rationale ||
                current == MicPermissionUi.Denied ||
                current == MicPermissionUi.PermanentlyDenied -> Unit
            else -> viewModel.onMicPermission(MicPermissionUi.Denied)
        }
    }

    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_RESUME -> {
                    viewModel.onActivityResumed(true)
                    refreshMicPermission()
                }
                Lifecycle.Event.ON_PAUSE -> viewModel.onActivityResumed(false)
                else -> Unit
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    LaunchedEffect(Unit) {
        // Request notification permission in context (does not block FGS).
        if (Build.VERSION.SDK_INT >= 33) {
            val granted = ContextCompat.checkSelfPermission(
                context,
                Manifest.permission.POST_NOTIFICATIONS,
            ) == PackageManager.PERMISSION_GRANTED
            if (granted) {
                viewModel.onNotificationPermission(NotificationPermissionUi.Granted)
            } else if (state.notificationPermission == NotificationPermissionUi.NotRequested) {
                notificationLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        } else {
            viewModel.onNotificationPermission(NotificationPermissionUi.Granted)
        }
        refreshMicPermission()
    }

    LiveSessionScreen(
        state = state,
        onBack = onBack,
        onJoin = {
            when (state.micPermission) {
                MicPermissionUi.Granted -> viewModel.join()
                MicPermissionUi.NotRequested, MicPermissionUi.Rationale, MicPermissionUi.Denied ->
                    micLauncher.launch(Manifest.permission.RECORD_AUDIO)
                MicPermissionUi.PermanentlyDenied, MicPermissionUi.RevokedLive -> {
                    val intent = Intent(
                        Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                        Uri.fromParts("package", context.packageName, null),
                    )
                    context.startActivity(intent)
                }
            }
        },
        onRejoin = {
            if (state.micPermission == MicPermissionUi.Granted) {
                viewModel.rejoin()
            } else {
                micLauncher.launch(Manifest.permission.RECORD_AUDIO)
            }
        },
        onStop = viewModel::stop,
        onToggleMute = {
            viewModel.setMuted(!state.service.micMuted)
        },
        onToggleDiagnostics = viewModel::toggleDiagnostics,
        onToggleQr = viewModel::toggleQr,
        onRetryQr = viewModel::retryBroadcastLink,
        onToggleRouteControls = viewModel::toggleRouteControls,
        onRetryRouteControls = viewModel::retryRouteControls,
        onAssignSource = viewModel::assignSource,
        onRemoveSource = viewModel::removeSource,
        onMuteSource = viewModel::setMixMuted,
        onSetSourceLevel = viewModel::setMixLevel,
        onRequestMicRationaleAck = {
            viewModel.onMicPermission(MicPermissionUi.Rationale)
            micLauncher.launch(Manifest.permission.RECORD_AUDIO)
        },
        onOpenSettings = {
            val intent = Intent(
                Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                Uri.fromParts("package", context.packageName, null),
            )
            context.startActivity(intent)
        },
        modifier = modifier,
    )
}

@Composable
fun LiveSessionScreen(
    state: LiveSessionUiState,
    onBack: () -> Unit,
    onJoin: () -> Unit,
    onRejoin: () -> Unit,
    onStop: () -> Unit,
    onToggleMute: () -> Unit,
    onToggleDiagnostics: () -> Unit,
    onToggleQr: () -> Unit = {},
    onRetryQr: () -> Unit = {},
    onToggleRouteControls: () -> Unit = {},
    onRetryRouteControls: () -> Unit = {},
    onAssignSource: (String, String) -> Unit = { _, _ -> },
    onRemoveSource: (String, String) -> Unit = { _, _ -> },
    onMuteSource: (String, String, Boolean) -> Unit = { _, _, _ -> },
    onSetSourceLevel: (String, String, Double) -> Unit = { _, _, _ -> },
    onRequestMicRationaleAck: () -> Unit = {},
    onOpenSettings: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing
    val service = state.service
    val live = service.isLiveOrConnecting
    val phase = service.phase

    val dominantLabel = when {
        state.requiresRejoin || phase == ServicePhase.ProcessRestored -> "Rejoin"
        phase == ServicePhase.Failed || phase == ServicePhase.Stopped -> "Join"
        live -> "Stop"
        else -> "Join"
    }
    val dominantIsStop = dominantLabel == "Stop"
    val statusTone = when (phase) {
        ServicePhase.Connected -> CtStatusTone.Ok
        ServicePhase.Muted -> CtStatusTone.Warning
        ServicePhase.Failed -> CtStatusTone.Danger
        ServicePhase.ReconnectScheduled, ServicePhase.WaitingForNetwork -> CtStatusTone.Warning
        ServicePhase.Preparing, ServicePhase.Minting, ServicePhase.Signaling, ServicePhase.IceChecking ->
            CtStatusTone.Info
        ServicePhase.ProcessRestored -> CtStatusTone.Warning
        else -> CtStatusTone.Neutral
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .padding(horizontal = spacing.gutter)
            .verticalScroll(rememberScrollState())
            .testTag("live_session_screen"),
    ) {
        Spacer(modifier = Modifier.height(spacing.space4))
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            CtButton(
                text = "Back",
                onClick = onBack,
                variant = CtButtonVariant.Ghost,
                testTag = "live_back",
            )
            CtStatus(
                text = service.notificationStatusLabel,
                tone = statusTone,
                testTag = "live_phase_status",
            )
        }

        Spacer(modifier = Modifier.height(spacing.space4))
        Text(
            text = "LIVE SESSION",
            style = type.eyebrow,
            color = colors.textTertiary,
        )
        Spacer(modifier = Modifier.height(spacing.space2))
        Text(
            text = state.sessionName,
            style = type.pageTitle,
            color = colors.textPrimary,
            modifier = Modifier
                .testTag("live_session_name")
                .semantics { contentDescription = "Session ${state.sessionName}" },
        )

        Spacer(modifier = Modifier.height(spacing.space3))
        Text(
            text = "Listening: ${state.feedName.ifBlank { "…" }}",
            style = type.body,
            color = colors.textSecondary,
            modifier = Modifier.testTag("live_feed_name"),
        )
        Text(
            text = "Speaking to: ${state.broadcastName.ifBlank { "…" }}",
            style = type.body,
            color = colors.textSecondary,
            modifier = Modifier.testTag("live_broadcast_name"),
        )

        Spacer(modifier = Modifier.height(spacing.space4))
        Text(
            text = state.statusSentence,
            style = type.lede,
            color = colors.textPrimary,
            modifier = Modifier
                .testTag("live_status_sentence")
                .semantics {
                    liveRegion = if (
                        phase == ServicePhase.Failed ||
                        phase == ServicePhase.ProcessRestored
                    ) {
                        LiveRegionMode.Assertive
                    } else {
                        LiveRegionMode.Polite
                    }
                    contentDescription = state.statusSentence
                },
        )

        if (state.notificationPermission == NotificationPermissionUi.Denied) {
            Spacer(modifier = Modifier.height(spacing.space2))
            Text(
                text = "Notification permission denied. Live audio continues, but the drawer notification may be hidden.",
                style = type.bodyCompact,
                color = colors.statusWarning,
                modifier = Modifier.testTag("notification_denied_warning"),
            )
        }

        if (state.channelsError != null) {
            Spacer(modifier = Modifier.height(spacing.space2))
            Text(
                text = state.channelsError,
                style = type.bodyCompact,
                color = colors.statusWarning,
                modifier = Modifier.testTag("live_channels_error"),
            )
        }

        if (state.micPermission == MicPermissionUi.PermanentlyDenied ||
            state.micPermission == MicPermissionUi.RevokedLive
        ) {
            Spacer(modifier = Modifier.height(spacing.space3))
            CtButton(
                text = "Open Settings",
                onClick = onOpenSettings,
                variant = CtButtonVariant.Secondary,
                fillMaxWidth = true,
                testTag = "mic_open_settings",
            )
        }

        Spacer(modifier = Modifier.height(spacing.space6))
        CtRule(strong = true)
        Spacer(modifier = Modifier.height(spacing.space4))

        // Audio workspace
        Text(
            text = "Audio",
            style = type.section,
            color = colors.textPrimary,
        )
        Spacer(modifier = Modifier.height(spacing.space3))
        CtMeter(
            label = "Mic",
            level = service.inputLevel,
            activeDescription = if (service.micMuted || service.captureSuppressed) {
                "Microphone muted"
            } else {
                "Mic active"
            },
            inactiveDescription = if (service.micMuted) "Microphone muted" else "No mic audio",
            testTag = "meter_mic",
        )
        Spacer(modifier = Modifier.height(spacing.space4))
        CtMeter(
            label = "Feed",
            level = service.outputLevel,
            activeDescription = "Feed audio",
            inactiveDescription = "No feed audio",
            testTag = "meter_feed",
        )

        Spacer(modifier = Modifier.height(spacing.space6))

        // Dominant Join/Stop/Rejoin
        CtButton(
            text = dominantLabel,
            onClick = {
                when (dominantLabel) {
                    "Stop" -> onStop()
                    "Rejoin" -> onRejoin()
                    else -> onJoin()
                }
            },
            variant = if (dominantIsStop) CtButtonVariant.Destructive else CtButtonVariant.Primary,
            fillMaxWidth = true,
            loading = state.channelsLoading && dominantLabel == "Join",
            enabled = when (dominantLabel) {
                "Join", "Rejoin" ->
                    !state.channelsLoading &&
                        state.micPermission != MicPermissionUi.PermanentlyDenied
                else -> true
            },
            testTag = "live_dominant_action",
        )

        Spacer(modifier = Modifier.height(spacing.space3))
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(spacing.space2),
        ) {
            CtButton(
                text = if (service.micMuted) "Unmute" else "Mute",
                onClick = onToggleMute,
                variant = CtButtonVariant.Secondary,
                enabled = live || phase == ServicePhase.Connected || phase == ServicePhase.Muted,
                modifier = Modifier.weight(1f),
                testTag = "live_mute",
            )
            CtButton(
                text = if (state.routeControlsVisible) "Hide route" else "Route",
                onClick = onToggleRouteControls,
                variant = CtButtonVariant.Secondary,
                enabled = !state.channelsLoading,
                modifier = Modifier.weight(1f),
                testTag = "live_route",
            )
        }

        if (state.routeControlsVisible) {
            Spacer(modifier = Modifier.height(spacing.space4))
            SessionRouteControls(
                channels = state.routeChannels,
                sources = state.routeSources,
                mixByChannel = state.mixByChannel,
                savingChannelIds = state.savingMixChannelIds,
                loading = state.routeControlsLoading,
                error = state.routeControlsError,
                onRetry = onRetryRouteControls,
                onAssign = onAssignSource,
                onRemove = onRemoveSource,
                onMute = onMuteSource,
                onLevel = onSetSourceLevel,
            )
        }

        Spacer(modifier = Modifier.height(spacing.space3))
        CtButton(
            text = when {
                state.qrVisible -> "Hide QR code"
                state.broadcastLinkError != null -> "Retry QR code"
                else -> "Show QR code"
            },
            onClick = if (state.broadcastListenerUrl != null) onToggleQr else onRetryQr,
            variant = CtButtonVariant.Ghost,
            fillMaxWidth = true,
            loading = state.broadcastLinkLoading,
            enabled = !state.broadcastLinkLoading &&
                (state.broadcastListenerUrl != null || state.broadcastLinkError != null),
            testTag = "live_qr_toggle",
        )
        state.broadcastLinkError?.let { message ->
            Spacer(modifier = Modifier.height(spacing.space2))
            Text(
                text = message,
                style = type.bodyCompact,
                color = colors.statusWarning,
                modifier = Modifier.testTag("live_qr_error"),
            )
        }
        if (state.qrVisible && state.broadcastListenerUrl != null) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("live_qr_panel"),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                CtQrCode(
                    value = state.broadcastListenerUrl,
                    contentDescription = "Broadcast listener QR code for ${state.sessionName}",
                    testTag = "live_broadcast_qr",
                )
                Spacer(modifier = Modifier.height(spacing.space2))
                Text(
                    text = "Share this code only with listeners for this session.",
                    style = type.metadata,
                    color = colors.textTertiary,
                )
            }
        }

        if (phase == ServicePhase.ReconnectScheduled || phase == ServicePhase.WaitingForNetwork) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Text(
                text = if (phase == ServicePhase.WaitingForNetwork) {
                    "Waiting for network before retry."
                } else {
                    "Reconnect scheduled${service.attemptCount.takeIf { it > 0 }?.let { " (attempt $it)" } ?: ""}."
                },
                style = type.bodyCompact,
                color = colors.statusWarning,
                modifier = Modifier.testTag("live_reconnect_banner"),
            )
        }

        Spacer(modifier = Modifier.height(spacing.space6))
        CtRule()
        Spacer(modifier = Modifier.height(spacing.space3))

        Text(
            text = if (state.diagnosticsExpanded) "Hide diagnostics" else "Show diagnostics",
            style = type.control,
            color = colors.accent,
            modifier = Modifier
                .clickable(onClick = onToggleDiagnostics)
                .padding(vertical = spacing.space2)
                .testTag("live_diagnostics_toggle")
                .semantics {
                    contentDescription = if (state.diagnosticsExpanded) {
                        "Hide diagnostics"
                    } else {
                        "Show diagnostics"
                    }
                },
        )

        if (state.diagnosticsExpanded) {
            CtMetadataBlock(title = "Diagnostics") {
                CtMetadata(label = "Phase", value = phase.name, monoValue = true)
                CtMetadata(label = "Session ID", value = state.sessionId, monoValue = true)
                CtMetadata(
                    label = "Generation",
                    value = service.generation.toString(),
                    monoValue = true,
                )
                service.stats?.let { stats ->
                    CtMetadata(
                        label = "In bytes",
                        value = stats.bytesReceived.toString(),
                        monoValue = true,
                    )
                    CtMetadata(
                        label = "Out bytes",
                        value = stats.bytesSent.toString(),
                        monoValue = true,
                    )
                }
                service.errorReason?.let {
                    CtMetadata(label = "Error", value = it, monoValue = false)
                }
            }
        }

        Spacer(modifier = Modifier.height(spacing.space8))
    }
}
