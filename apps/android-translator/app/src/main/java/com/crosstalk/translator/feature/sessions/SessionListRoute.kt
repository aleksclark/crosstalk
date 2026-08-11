package com.crosstalk.translator.feature.sessions

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.crosstalk.translator.contract.SessionSummary

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
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(16.dp)
            .testTag("session_list_screen"),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = "Assigned sessions",
                    style = MaterialTheme.typography.titleLarge,
                )
                if (!state.username.isNullOrBlank()) {
                    Text(
                        text = state.username,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag("signed_in_username"),
                    )
                }
            }
            TextButton(
                onClick = onLogout,
                modifier = Modifier.testTag("logout_button"),
            ) {
                Text("Log out")
            }
        }

        Spacer(modifier = Modifier.height(8.dp))
        Button(
            onClick = onRefresh,
            enabled = !state.isLoading,
            modifier = Modifier.testTag("refresh_sessions"),
        ) {
            Text("Refresh")
        }

        if (state.errorMessage != null) {
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = state.errorMessage,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("session_list_error"),
            )
        }

        Spacer(modifier = Modifier.height(12.dp))

        when {
            state.isLoading && state.sessions.isEmpty() -> {
                CircularProgressIndicator(
                    modifier = Modifier
                        .align(Alignment.CenterHorizontally)
                        .testTag("session_list_progress"),
                )
            }
            state.sessions.isEmpty() -> {
                Text(
                    text = "No assigned sessions",
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.testTag("session_list_empty"),
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
                        HorizontalDivider()
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
    var expanded by remember(session.id) { mutableStateOf(false) }
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(vertical = 12.dp)
            .testTag("session_row_${session.id}")
            .semantics {
                contentDescription = "Session ${session.name}"
            },
    ) {
        // Name is always primary; never use truncated ULID as title.
        Text(
            text = session.name,
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.testTag("session_name_${session.id}"),
        )
        val secondary = listOfNotNull(
            session.description?.takeIf { it.isNotBlank() },
            session.status?.takeIf { it.isNotBlank() },
        ).joinToString(" · ")
        if (secondary.isNotBlank()) {
            Text(
                text = secondary,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        TextButton(
            onClick = { expanded = !expanded },
            modifier = Modifier.testTag("session_id_toggle_${session.id}"),
        ) {
            Text(if (expanded) "Hide ID" else "Show ID")
        }
        if (expanded) {
            Text(
                text = session.id,
                style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace),
                modifier = Modifier
                    .testTag("session_id_${session.id}")
                    .semantics {
                        contentDescription = "Session identifier ${session.id}"
                    },
            )
        }
    }
}
