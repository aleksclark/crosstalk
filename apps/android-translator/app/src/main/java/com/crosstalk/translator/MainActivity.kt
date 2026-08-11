package com.crosstalk.translator

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.crosstalk.translator.app.AppContainer
import com.crosstalk.translator.auth.AuthState
import com.crosstalk.translator.feature.login.LoginRoute
import com.crosstalk.translator.feature.login.LoginViewModel
import com.crosstalk.translator.feature.sessions.SessionListRoute
import com.crosstalk.translator.feature.sessions.SessionListViewModel

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val container = (application as CrossTalkApplication).container
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    TranslatorNavHost(container = container)
                }
            }
        }
    }
}

private object Routes {
    const val BOOT = "boot"
    const val LOGIN = "login"
    const val SESSIONS = "sessions"
    const val LIVE = "live/{sessionId}"
    fun live(sessionId: String): String = "live/$sessionId"
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
            Box(
                modifier = Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) {
                Text(text = if (bootDone) "Ready" else "Restoring…")
            }
        }
        composable(Routes.LOGIN) {
            val loginVm: LoginViewModel = viewModel(
                factory = LoginViewModel.Factory(
                    authRepository = container.authRepository,
                    deploymentIdentity = container.deploymentIdentity,
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
                    navController.navigate(Routes.live(session.id))
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
            arguments = listOf(navArgument("sessionId") { type = NavType.StringType }),
        ) { entry ->
            val sessionId = entry.arguments?.getString("sessionId").orEmpty()
            // Phase 2 placeholder; live WebRTC arrives in later phases.
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(24.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text(text = "Live session placeholder\n$sessionId")
            }
        }
    }
}
