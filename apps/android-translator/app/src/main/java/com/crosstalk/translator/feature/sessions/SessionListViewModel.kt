package com.crosstalk.translator.feature.sessions

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.SessionSummary
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SessionListUiState(
    val username: String? = null,
    val sessions: List<SessionSummary> = emptyList(),
    val isLoading: Boolean = false,
    val errorMessage: String? = null,
    val loggedOut: Boolean = false,
)

class SessionListViewModel(
    private val authRepository: AuthRepository,
) : ViewModel() {
    private val _uiState = MutableStateFlow(
        SessionListUiState(username = authRepository.username),
    )
    val uiState: StateFlow<SessionListUiState> = _uiState.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    isLoading = true,
                    errorMessage = null,
                    username = authRepository.username,
                )
            }
            try {
                val sessions = authRepository.listAssignedSessions()
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        sessions = sessions,
                        errorMessage = null,
                        username = authRepository.username,
                    )
                }
            } catch (e: ApiException.Unauthorized) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        sessions = emptyList(),
                        errorMessage = "Session expired. Sign in again.",
                        loggedOut = true,
                    )
                }
            } catch (e: ApiException.Network) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        errorMessage = "Unable to load assignments. Check network.",
                    )
                }
            } catch (e: Exception) {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        errorMessage = e.message?.takeIf { msg -> msg.isNotBlank() }
                            ?: "Unable to load assignments",
                    )
                }
            }
        }
    }

    fun logout() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null) }
            try {
                authRepository.logout()
            } finally {
                _uiState.update {
                    it.copy(
                        isLoading = false,
                        sessions = emptyList(),
                        loggedOut = true,
                        username = null,
                    )
                }
            }
        }
    }

    fun consumeLoggedOut() {
        _uiState.update { it.copy(loggedOut = false) }
    }

    class Factory(
        private val authRepository: AuthRepository,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            require(modelClass.isAssignableFrom(SessionListViewModel::class.java))
            return SessionListViewModel(authRepository) as T
        }
    }
}
