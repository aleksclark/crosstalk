package com.crosstalk.translator.feature.login

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.AuthState
import com.crosstalk.translator.contract.ApiException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class LoginUiState(
    val serverUrl: String = "",
    val username: String = "",
    val password: String = "",
    val isLoading: Boolean = false,
    val errorMessage: String? = null,
    val serverErrorMessage: String? = null,
    val signedInUsername: String? = null,
)

class LoginViewModel(
    private val authRepositoryProvider: suspend (String) -> AuthRepository,
    initialServerUrl: String,
) : ViewModel() {
    private val _uiState = MutableStateFlow(
        LoginUiState(
            serverUrl = initialServerUrl,
        ),
    )
    val uiState: StateFlow<LoginUiState> = _uiState.asStateFlow()

    fun onServerUrlChange(value: String) {
        _uiState.update {
            it.copy(
                serverUrl = value,
                errorMessage = null,
                serverErrorMessage = null,
            )
        }
    }

    fun onUsernameChange(value: String) {
        _uiState.update { it.copy(username = value, errorMessage = null) }
    }

    fun onPasswordChange(value: String) {
        _uiState.update { it.copy(password = value, errorMessage = null) }
    }

    fun submit() {
        val current = _uiState.value
        if (current.serverUrl.isBlank()) {
            _uiState.update { it.copy(serverErrorMessage = "Enter a server URL") }
            return
        }
        val username = current.username.trim()
        val password = current.password
        if (username.isEmpty() || password.isEmpty()) {
            _uiState.update {
                it.copy(errorMessage = "Enter username and password")
            }
            return
        }
        if (current.isLoading) return

        viewModelScope.launch {
            _uiState.update {
                it.copy(isLoading = true, errorMessage = null)
            }
            try {
                val authRepository = authRepositoryProvider(current.serverUrl)
                val state = authRepository.login(username, password)
                val signedInUser = (state as? AuthState.SignedIn)?.username ?: username
                // Clear password from UI state immediately after success.
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        password = "",
                        errorMessage = null,
                        signedInUsername = signedInUser,
                    )
                }
            } catch (e: IllegalArgumentException) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        serverErrorMessage = e.message ?: "Enter a valid server URL",
                    )
                }
            } catch (e: ApiException.Unauthorized) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        password = "",
                        errorMessage = "Invalid username or password",
                        signedInUsername = null,
                    )
                }
            } catch (e: ApiException.Network) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        password = "",
                        errorMessage = "Server unavailable. Check network and try again.",
                        signedInUsername = null,
                    )
                }
            } catch (e: ApiException.Server) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        password = "",
                        errorMessage = "Server error. Try again later.",
                        signedInUsername = null,
                    )
                }
            } catch (e: Exception) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        password = "",
                        errorMessage = e.message?.takeIf { msg -> msg.isNotBlank() }
                            ?: "Sign-in failed",
                        signedInUsername = null,
                    )
                }
            }
        }
    }

    fun consumeSignedIn() {
        _uiState.update { it.copy(signedInUsername = null) }
    }

    class Factory(
        private val authRepositoryProvider: suspend (String) -> AuthRepository,
        private val initialServerUrl: String,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(LoginViewModel::class.java))
            return LoginViewModel(authRepositoryProvider, initialServerUrl) as T
        }
    }
}
