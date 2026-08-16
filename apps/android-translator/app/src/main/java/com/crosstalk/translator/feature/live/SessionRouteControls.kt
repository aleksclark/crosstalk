package com.crosstalk.translator.feature.live

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.MixEntry
import com.crosstalk.translator.contract.SourceInfo
import com.crosstalk.translator.ui.components.CtButton
import com.crosstalk.translator.ui.components.CtButtonVariant
import com.crosstalk.translator.ui.components.CtRule
import com.crosstalk.translator.ui.theme.CtTheme
import kotlin.math.roundToInt

@Composable
fun SessionRouteControls(
    channels: List<ChannelInfo>,
    sources: List<SourceInfo>,
    mixByChannel: Map<String, List<MixEntry>>,
    savingChannelIds: Set<String>,
    loading: Boolean,
    error: String?,
    onRetry: () -> Unit,
    onAssign: (channelId: String, sourceId: String) -> Unit,
    onRemove: (channelId: String, sourceId: String) -> Unit,
    onMute: (channelId: String, sourceId: String, muted: Boolean) -> Unit,
    onLevel: (channelId: String, sourceId: String, level: Double) -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing
    val sourceById = sources.associateBy { it.id }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .testTag("route_controls"),
    ) {
        CtRule(strong = true)
        Spacer(modifier = Modifier.height(spacing.space4))
        Text(
            text = "Session channels",
            style = type.section,
            color = colors.textPrimary,
        )
        Spacer(modifier = Modifier.height(spacing.space2))
        Text(
            text = "Choose the sources feeding each channel. Changes apply to the live session.",
            style = type.bodyCompact,
            color = colors.textSecondary,
        )

        if (loading) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Text(
                text = "Loading channel controls…",
                style = type.body,
                color = colors.textTertiary,
                modifier = Modifier.testTag("route_controls_loading"),
            )
            return@Column
        }

        if (error != null) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Text(
                text = error,
                style = type.bodyCompact,
                color = colors.statusDanger,
                modifier = Modifier.testTag("route_controls_error"),
            )
            Spacer(modifier = Modifier.height(spacing.space2))
            CtButton(
                text = "Retry",
                onClick = onRetry,
                variant = CtButtonVariant.Secondary,
                testTag = "route_controls_retry",
            )
        }

        if (channels.isEmpty() && error == null) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Text(
                text = "No channels are configured for this session.",
                style = type.body,
                color = colors.textTertiary,
                modifier = Modifier.testTag("route_controls_empty"),
            )
            return@Column
        }

        channels.forEach { channel ->
            val entries = mixByChannel[channel.id].orEmpty()
            val assignedSourceIds = entries.mapTo(mutableSetOf()) { it.sourceId }
            val available = sources.filterNot { it.id in assignedSourceIds }
            val saving = channel.id in savingChannelIds

            Spacer(modifier = Modifier.height(spacing.space6))
            CtRule()
            Spacer(modifier = Modifier.height(spacing.space3))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = channel.name.ifBlank { "Unnamed channel" },
                        style = type.control,
                        color = colors.textPrimary,
                        modifier = Modifier.testTag("route_channel_${channel.id}"),
                    )
                    Text(
                        text = channel.type.uppercase(),
                        style = type.metadata,
                        color = colors.textTertiary,
                    )
                }
                if (saving) {
                    Text(
                        text = "Saving…",
                        style = type.metadata,
                        color = colors.accent,
                        modifier = Modifier.testTag("route_saving_${channel.id}"),
                    )
                }
            }

            if (entries.isEmpty()) {
                Spacer(modifier = Modifier.height(spacing.space2))
                Text(
                    text = "No sources assigned.",
                    style = type.bodyCompact,
                    color = colors.textTertiary,
                )
            }

            entries.forEach { entry ->
                val source = sourceById[entry.sourceId]
                Spacer(modifier = Modifier.height(spacing.space3))
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(start = spacing.space2)
                        .testTag("route_mix_${channel.id}_${entry.sourceId}"),
                ) {
                    Text(
                        text = source?.name?.ifBlank { null } ?: "Unknown source",
                        style = type.body,
                        color = colors.textPrimary,
                    )
                    Text(
                        text = sourceDescription(source),
                        style = type.metadata,
                        color = if (source?.connected == true) colors.statusOk else colors.textTertiary,
                    )
                    Spacer(modifier = Modifier.height(spacing.space2))
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(spacing.space2),
                    ) {
                        CtButton(
                            text = if (entry.muted) "Unmute" else "Mute",
                            onClick = { onMute(channel.id, entry.sourceId, !entry.muted) },
                            variant = CtButtonVariant.Secondary,
                            modifier = Modifier.weight(1f),
                            testTag = "route_mute_${channel.id}_${entry.sourceId}",
                        )
                        CtButton(
                            text = "Remove",
                            onClick = { onRemove(channel.id, entry.sourceId) },
                            variant = CtButtonVariant.Ghost,
                            modifier = Modifier.weight(1f),
                            testTag = "route_remove_${channel.id}_${entry.sourceId}",
                        )
                    }
                    Spacer(modifier = Modifier.height(spacing.space2))
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(spacing.space2),
                    ) {
                        CtButton(
                            text = "−",
                            onClick = { onLevel(channel.id, entry.sourceId, entry.level - LEVEL_STEP) },
                            variant = CtButtonVariant.Ghost,
                            enabled = entry.level > MIN_LEVEL,
                            testTag = "route_level_down_${channel.id}_${entry.sourceId}",
                        )
                        Text(
                            text = "${(entry.level * 100).roundToInt()}%",
                            style = type.metadata,
                            color = colors.textSecondary,
                            modifier = Modifier
                                .weight(1f)
                                .padding(vertical = spacing.space3)
                                .testTag("route_level_${channel.id}_${entry.sourceId}"),
                        )
                        CtButton(
                            text = "+",
                            onClick = { onLevel(channel.id, entry.sourceId, entry.level + LEVEL_STEP) },
                            variant = CtButtonVariant.Ghost,
                            enabled = entry.level < MAX_LEVEL,
                            testTag = "route_level_up_${channel.id}_${entry.sourceId}",
                        )
                    }
                }
            }

            if (available.isNotEmpty()) {
                Spacer(modifier = Modifier.height(spacing.space3))
                Text(
                    text = "Available sources",
                    style = type.label,
                    color = colors.textSecondary,
                )
                available.forEach { source ->
                    Spacer(modifier = Modifier.height(spacing.space2))
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(start = spacing.space2),
                        horizontalArrangement = Arrangement.spacedBy(spacing.space2),
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = source.name.ifBlank { "Unknown source" },
                                style = type.bodyCompact,
                                color = colors.textPrimary,
                            )
                            Text(
                                text = sourceDescription(source),
                                style = type.metadata,
                                color = colors.textTertiary,
                            )
                        }
                        CtButton(
                            text = "Add",
                            onClick = { onAssign(channel.id, source.id) },
                            variant = CtButtonVariant.Secondary,
                            testTag = "route_add_${channel.id}_${source.id}",
                        )
                    }
                }
            }
        }
    }
}

private fun sourceDescription(source: SourceInfo?): String = when {
    source == null -> "Source unavailable"
    source.connected -> "${source.origin} · connected"
    else -> "${source.origin} · offline"
}

private const val MIN_LEVEL = 0.0
private const val MAX_LEVEL = 2.0
private const val LEVEL_STEP = 0.1
