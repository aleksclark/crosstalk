package com.crosstalk.translator

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.crosstalk.translator.app.AppContainer
import com.crosstalk.translator.auth.AuthState
import com.crosstalk.translator.feature.live.LiveSessionRoute
import com.crosstalk.translator.feature.live.LiveSessionViewModel
import com.crosstalk.translator.feature.login.LoginRoute
import com.crosstalk.translator.feature.login.LoginViewModel
import com.crosstalk.translator.feature.sessions.SessionListRoute
import com.crosstalk.translator.feature.sessions.SessionListViewModel
import com.crosstalk.translator.ui.theme.CrossTalkTheme
import com.crosstalk.translator.ui.theme.CtTheme
import java.net.URLDecoder
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

class MainActivity : ComponentActivity() {
    private var container: AppContainer? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val appContainer = (application as CrossTalkApplication).container
        container = appContainer
        // Bind early so process-restored service state is visible before Join.
        appContainer.audioServiceGateway.bind()
        setContent {
            CrossTalkTheme {
                TranslatorNavHost(container = appContainer)
            }
        }
    }

    override fun onDestroy() {
        // Keep service bound only while activity process lives; unbind on destroy
        // does not stop a live FGS (stopWithTask=false).
        if (isFinishing) {
            container?.audioServiceGateway?.unbind()
        }
        super.onDestroy()
    }
}

private object Routes {
    const val BOOT = "boot"
    const val LOGIN = "login"
    const val SESSIONS = "sessions"
    const val LIVE = "live/{sessionId}?name={sessionName}"
    fun live(sessionId: String, sessionName: String): String {
        val encodedName = URLEncoder.encode(sessionName, StandardCharsets.UTF_8.toString())
        val encodedId = URLEncoder.encode(sessionId, StandardCharsets.UTF_8.toString())
        return "live/$encodedId?name=$encodedName"
    }
}

@Composable
private fun TranslatorNavHost(container: AppContainer) {
    val navController = rememberNavController()
    var bootDone by remember { mutableStateOf(false) }
    var startDestination by remember { mutableStateOf(Routes.BOOT) }

    LaunchedEffect(Unit) {
        val state = container.authRepository.restoreSession()
        startDestination = when (state) {
            is AuthState.SignedIn -> Routes.SESSIONS
            else -> Routes.LOGIN
        }
        bootDone = true
        navController.navigate(startDestination) {
            popUpTo(Routes.BOOT) { inclusive = true }
        }
    }

    NavHost(
        navController = navController,
        startDestination = Routes.BOOT,
    ) {
        composable(Routes.BOOT) {
            val colors = CtTheme.colors
            val type = CtTheme.typography
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .testTag("boot_screen"),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    text = if (bootDone) "Ready" else "Restoring…",
                    style = type.body,
                    color = colors.textSecondary,
                )
            }
        }
        composable(Routes.LOGIN) {
            val loginVm: LoginViewModel = viewModel(
                factory = LoginViewModel.Factory(
                    authRepositoryProvider = container::configureServer,
                    initialServerUrl = container.apiBaseUrl,
                ),
            )
            LoginRoute(
                viewModel = loginVm,
                onSignedIn = {
                    navController.navigate(Routes.SESSIONS) {
                        popUpTo(Routes.LOGIN) { inclusive = true }
                    }
                },
            )
        }
        composable(Routes.SESSIONS) {
            val sessionsVm: SessionListViewModel = viewModel(
                factory = SessionListViewModel.Factory(
                    authRepository = container.authRepository,
                ),
            )
            SessionListRoute(
                viewModel = sessionsVm,
                onSessionSelected = { session ->
                    navController.navigate(Routes.live(session.id, session.name))
                },
                onLoggedOut = {
                    navController.navigate(Routes.LOGIN) {
                        popUpTo(Routes.SESSIONS) { inclusive = true }
                    }
                },
            )
        }
        composable(
            route = Routes.LIVE,
            arguments = listOf(
                navArgument("sessionId") { type = NavType.StringType },
                navArgument("sessionName") {
                    type = NavType.StringType
                    defaultValue = ""
                },
            ),
        ) { entry ->
            val rawId = entry.arguments?.getString("sessionId").orEmpty()
            val rawName = entry.arguments?.getString("sessionName").orEmpty()
            val sessionId = URLDecoder.decode(rawId, StandardCharsets.UTF_8.toString())
            val sessionName = URLDecoder.decode(rawName, StandardCharsets.UTF_8.toString())
                .ifBlank { sessionId }

            val liveVm: LiveSessionViewModel = viewModel(
                factory = LiveSessionViewModel.Factory(
                    sessionId = sessionId,
                    sessionName = sessionName,
                    api = container.api,
                    gateway = container.audioServiceGateway,
                ),
            )
            // System Back backgrounds without Stop — only pop the nav stack.
            LiveSessionRoute(
                viewModel = liveVm,
                onBack = { navController.popBackStack() },
            )
        }
    }
}
