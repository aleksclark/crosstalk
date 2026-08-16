package com.crosstalk.translator.feature.login

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.crosstalk.translator.ui.components.CtButton
import com.crosstalk.translator.ui.components.CtButtonVariant
import com.crosstalk.translator.ui.components.CtField
import com.crosstalk.translator.ui.components.CtRule
import com.crosstalk.translator.ui.theme.CtTheme

@Composable
fun LoginRoute(
    viewModel: LoginViewModel,
    onSignedIn: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(state.signedInUsername) {
        if (state.signedInUsername != null) {
            viewModel.consumeSignedIn()
            onSignedIn()
        }
    }

    LoginScreen(
        state = state,
        onServerUrlChange = viewModel::onServerUrlChange,
        onUsernameChange = viewModel::onUsernameChange,
        onPasswordChange = viewModel::onPasswordChange,
        onSubmit = viewModel::submit,
        modifier = modifier,
    )
}

@Composable
fun LoginScreen(
    state: LoginUiState,
    onServerUrlChange: (String) -> Unit,
    onUsernameChange: (String) -> Unit,
    onPasswordChange: (String) -> Unit,
    onSubmit: () -> Unit,
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
            .verticalScroll(rememberScrollState())
            .testTag("login_screen"),
    ) {
        Spacer(modifier = Modifier.height(spacing.space8))
        Text(
            text = "CONFIGURE",
            style = type.eyebrow,
            color = colors.textTertiary,
        )
        Spacer(modifier = Modifier.height(spacing.space2))
        Text(
            text = "Welcome",
            style = type.editorialWelcome,
            color = colors.textPrimary,
            modifier = Modifier.testTag("login_welcome"),
        )
        Spacer(modifier = Modifier.height(spacing.space2))
        Text(
            text = "CrossTalk Translator",
            style = type.pageTitle,
            color = colors.textPrimary,
        )
        Spacer(modifier = Modifier.height(spacing.space6))
        CtRule()
        Spacer(modifier = Modifier.height(spacing.space6))

        CtField(
            label = "Server URL",
            value = state.serverUrl,
            onValueChange = onServerUrlChange,
            help = "Use the HTTPS address for your CrossTalk server",
            error = state.serverErrorMessage,
            enabled = !state.isLoading,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Uri,
                imeAction = ImeAction.Next,
            ),
            testTag = "login_server_url",
        )
        Spacer(modifier = Modifier.height(spacing.space4))
        CtField(
            label = "Username",
            value = state.username,
            onValueChange = onUsernameChange,
            enabled = !state.isLoading,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Text,
                imeAction = ImeAction.Next,
            ),
            testTag = "login_username",
        )
        Spacer(modifier = Modifier.height(spacing.space4))
        CtField(
            label = "Password",
            value = state.password,
            onValueChange = onPasswordChange,
            enabled = !state.isLoading,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Password,
                imeAction = ImeAction.Done,
            ),
            keyboardActions = KeyboardActions(onDone = { onSubmit() }),
            error = state.errorMessage,
            testTag = "login_password",
        )
        if (state.errorMessage != null) {
            // Stable tag for existing instrumentation / semantics tests.
            Text(
                text = state.errorMessage,
                style = type.bodyCompact,
                color = colors.statusDanger,
                modifier = Modifier
                    .testTag("login_error")
                    .semantics { contentDescription = state.errorMessage },
            )
        }

        Spacer(modifier = Modifier.height(spacing.space6))
        CtButton(
            text = if (state.isLoading) "Signing in…" else "Sign in",
            onClick = onSubmit,
            variant = CtButtonVariant.Primary,
            fillMaxWidth = true,
            loading = state.isLoading,
            enabled = !state.isLoading,
            testTag = "login_submit",
        )
        if (state.isLoading) {
            Spacer(modifier = Modifier.height(spacing.space2))
            Text(
                text = "Signing in",
                style = type.metadata,
                color = colors.textTertiary,
                modifier = Modifier.testTag("login_progress"),
            )
        }

        Spacer(modifier = Modifier.height(spacing.space12))
    }
}
