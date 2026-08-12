package com.crosstalk.translator.feature.sessions

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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.crosstalk.translator.contract.SessionSummary
import com.crosstalk.translator.ui.components.CtButton
import com.crosstalk.translator.ui.components.CtButtonVariant
import com.crosstalk.translator.ui.components.CtRule
import com.crosstalk.translator.ui.components.CtStatus
import com.crosstalk.translator.ui.components.CtStatusTone
import com.crosstalk.translator.ui.theme.CtTheme

@Composable
fun SessionListRoute(
    viewModel: SessionListViewModel,
    onSessionSelected: (SessionSummary) -> Unit,
    onLoggedOut: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(state.loggedOut) {
        if (state.loggedOut) {
            viewModel.consumeLoggedOut()
            onLoggedOut()
        }
    }

    SessionListScreen(
        state = state,
        onRefresh = viewModel::refresh,
        onLogout = viewModel::logout,
        onSessionSelected = onSessionSelected,
        modifier = modifier,
    )
}

@Composable
fun SessionListScreen(
    state: SessionListUiState,
    onRefresh: () -> Unit,
    onLogout: () -> Unit,
    onSessionSelected: (SessionSummary) -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing

    Column(
        modifier = modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .padding(horizontal = spacing.gutter)
            .testTag("session_list_screen"),
    ) {
        Spacer(modifier = Modifier.height(spacing.space4))
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = "ASSIGNMENTS",
                    style = type.eyebrow,
                    color = colors.textTertiary,
                )
                Spacer(modifier = Modifier.height(spacing.space1))
                Text(
                    text = "Assigned sessions",
                    style = type.pageTitle,
                    color = colors.textPrimary,
                )
                if (!state.username.isNullOrBlank()) {
                    Spacer(modifier = Modifier.height(spacing.space1))
                    Text(
                        text = state.username,
                        style = type.body,
                        color = colors.textSecondary,
                        modifier = Modifier.testTag("signed_in_username"),
                    )
                }
            }
            CtButton(
                text = "Log out",
                onClick = onLogout,
                variant = CtButtonVariant.Ghost,
                testTag = "logout_button",
            )
        }

        Spacer(modifier = Modifier.height(spacing.space4))
        CtButton(
            text = if (state.isLoading) "Refreshing…" else "Refresh",
            onClick = onRefresh,
            variant = CtButtonVariant.Secondary,
            enabled = !state.isLoading,
            testTag = "refresh_sessions",
        )

        if (state.errorMessage != null) {
            Spacer(modifier = Modifier.height(spacing.space3))
            Text(
                text = state.errorMessage,
                style = type.body,
                color = colors.statusDanger,
                modifier = Modifier.testTag("session_list_error"),
            )
        }

        Spacer(modifier = Modifier.height(spacing.space4))
        CtRule(strong = true)
        Spacer(modifier = Modifier.height(spacing.space2))

        when {
            state.isLoading && state.sessions.isEmpty() -> {
                Spacer(modifier = Modifier.height(spacing.space6))
                Text(
                    text = "Loading assignments…",
                    style = type.body,
                    color = colors.textSecondary,
                    modifier = Modifier.testTag("session_list_progress"),
                )
            }
            state.sessions.isEmpty() -> {
                Spacer(modifier = Modifier.height(spacing.space6))
                Text(
                    text = "No assigned sessions",
                    style = type.section,
                    color = colors.textPrimary,
                    modifier = Modifier.testTag("session_list_empty"),
                )
                Spacer(modifier = Modifier.height(spacing.space2))
                Text(
                    text = "When an administrator assigns you to a session, it will appear here.",
                    style = type.body,
                    color = colors.textSecondary,
                )
            }
            else -> {
                LazyColumn(
                    modifier = Modifier
                        .fillMaxSize()
                        .testTag("session_list"),
                ) {
                    items(state.sessions, key = { it.id }) { session ->
                        SessionRow(
                            session = session,
                            onClick = { onSessionSelected(session) },
                        )
                        CtRule()
                    }
                }
            }
        }
    }
}

@Composable
private fun SessionRow(
    session: SessionSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val colors = CtTheme.colors
    val type = CtTheme.typography
    val spacing = CtTheme.spacing
    var expanded by remember(session.id) { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(vertical = spacing.space3)
            .testTag("session_row_${session.id}")
            .semantics {
                contentDescription = "Session ${session.name}"
            },
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = session.name,
                style = type.section,
                color = colors.textPrimary,
                modifier = Modifier
                    .weight(1f)
                    .testTag("session_name_${session.id}"),
            )
            session.status?.takeIf { it.isNotBlank() }?.let { status ->
                CtStatus(
                    text = status,
                    tone = CtStatusTone.Neutral,
                )
            }
        }
        val description = session.description?.takeIf { it.isNotBlank() }
        if (description != null) {
            Spacer(modifier = Modifier.height(spacing.space1))
            Text(
                text = description,
                style = type.bodyCompact,
                color = colors.textSecondary,
            )
        }
        Spacer(modifier = Modifier.height(spacing.space1))
        Text(
            text = if (expanded) "Hide ID" else "Show ID",
            style = type.control,
            color = colors.accent,
            modifier = Modifier
                .clickable { expanded = !expanded }
                .padding(vertical = spacing.space1)
                .testTag("session_id_toggle_${session.id}"),
        )
        if (expanded) {
            Text(
                text = session.id,
                style = type.code,
                color = colors.textTertiary,
                modifier = Modifier
                    .testTag("session_id_${session.id}")
                    .semantics {
                        contentDescription = "Session identifier ${session.id}"
                    },
            )
        }
    }
}
