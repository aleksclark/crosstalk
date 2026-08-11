package com.crosstalk.translator.auth

import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.AuthTokens
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.GeneratedApiAdapter
import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.contract.SessionSummary
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.atomic.AtomicReference

/**
 * Owns access JWT (memory only), encrypted refresh persistence, refresh
 * deduplication, and a single 401 refresh-and-replay for idempotent reads/mint.
 */
class AuthRepository(
    private val api: CrossTalkApi,
    private val vault: CredentialVault,
    private val accessTokenSink: (String?) -> Unit = { token ->
        (api as? GeneratedApiAdapter)?.setAccessToken(token)
    },
) {
    private val stateMutex = Mutex()
    private val refreshMutex = Mutex()
    private var inFlightRefresh: CompletableDeferred<AuthTokens>? = null

    private val accessTokenRef = AtomicReference<String?>(null)
    private val refreshTokenRef = AtomicReference<String?>(null)
    private val usernameRef = AtomicReference<String?>(null)

    private val _authState = MutableStateFlow<AuthState>(AuthState.Unknown)
    val authState: StateFlow<AuthState> = _authState.asStateFlow()

    val username: String? get() = usernameRef.get()

    suspend fun restoreSession(): AuthState {
        _authState.value = AuthState.Restoring
        val stored = vault.readRefreshToken()
        if (stored.isNullOrBlank()) {
            clearMemory()
            _authState.value = AuthState.SignedOut
            return AuthState.SignedOut
        }
        refreshTokenRef.set(stored)
        return try {
            val tokens = refreshInternal(stored)
            applyTokens(tokens, previousUsername = null)
            val signedIn = AuthState.SignedIn(username = usernameRef.get())
            _authState.value = signedIn
            signedIn
        } catch (e: ApiException.Unauthorized) {
            clearLocalCredentials()
            _authState.value = AuthState.SignedOut
            AuthState.SignedOut
        } catch (e: ApiException.Client) {
            if (e.statusCode == 401 || e.statusCode == 403) {
                clearLocalCredentials()
                _authState.value = AuthState.SignedOut
                AuthState.SignedOut
            } else {
                // Transient client/server issues: keep vault, report signed out for UI.
                clearMemory()
                _authState.value = AuthState.SignedOut
                AuthState.SignedOut
            }
        } catch (_: Exception) {
            clearMemory()
            _authState.value = AuthState.SignedOut
            AuthState.SignedOut
        }
    }

    suspend fun login(username: String, password: String): AuthState {
        val trimmedUser = username.trim()
        require(trimmedUser.isNotEmpty()) { "username must not be blank" }
        require(password.isNotEmpty()) { "password must not be blank" }

        _authState.value = AuthState.Authenticating
        return try {
            val tokens = api.login(trimmedUser, password)
            applyTokens(tokens, previousUsername = trimmedUser)
            val signedIn = AuthState.SignedIn(username = trimmedUser)
            _authState.value = signedIn
            signedIn
        } catch (e: ApiException.Unauthorized) {
            clearMemory()
            _authState.value = AuthState.SignedOut
            throw e
        } catch (e: ApiException.Client) {
            clearMemory()
            _authState.value = AuthState.SignedOut
            throw e
        } catch (e: Exception) {
            clearMemory()
            _authState.value = AuthState.SignedOut
            throw e
        }
    }

    /**
     * Best-effort server logout, then always clear local credentials.
     */
    suspend fun logout() {
        val refresh = refreshTokenRef.get()
        try {
            if (!refresh.isNullOrBlank()) {
                api.logout(refresh)
            }
        } catch (_: Exception) {
            // Network/server failure must not retain local credentials.
        } finally {
            clearLocalCredentials()
            _authState.value = AuthState.SignedOut
        }
    }

    suspend fun listAssignedSessions(): List<SessionSummary> =
        withAuthRetry { api.listSessions() }

    suspend fun getSession(sessionId: String): SessionSummary =
        withAuthRetry { api.getSession(sessionId) }

    suspend fun listChannels(sessionId: String): List<ChannelInfo> =
        withAuthRetry { api.listChannels(sessionId) }

    suspend fun mintMediaTicket(sessionId: String): MediaTicket =
        withAuthRetry { api.mintMediaTicket(sessionId = sessionId) }

    /**
     * Returns a valid access token, refreshing once if needed.
     * Used by the foreground service before minting.
     */
    suspend fun requireAccessToken(): String {
        val current = accessTokenRef.get()
        if (!current.isNullOrBlank()) return current
        val refresh = refreshTokenRef.get()
            ?: vault.readRefreshToken()
            ?: throw ApiException.Unauthorized("No credentials")
        val tokens = refreshInternal(refresh)
        applyTokens(tokens, previousUsername = usernameRef.get())
        return tokens.accessToken
    }

    private suspend fun <T> withAuthRetry(block: suspend () -> T): T {
        ensureAccessTokenPresent()
        return try {
            block()
        } catch (e: ApiException.Unauthorized) {
            // Single refresh-and-replay for idempotent reads / mint only.
            val refresh = refreshTokenRef.get()
                ?: vault.readRefreshToken()
                ?: run {
                    clearLocalCredentials()
                    _authState.value = AuthState.SignedOut
                    throw e
                }
            try {
                val tokens = refreshInternal(refresh)
                applyTokens(tokens, previousUsername = usernameRef.get())
            } catch (refreshError: Exception) {
                clearLocalCredentials()
                _authState.value = AuthState.SignedOut
                throw when (refreshError) {
                    is ApiException -> refreshError
                    else -> ApiException.Unauthorized(
                        message = refreshError.message ?: "Refresh failed",
                        cause = refreshError,
                    )
                }
            }
            block()
        }
    }

    private suspend fun ensureAccessTokenPresent() {
        if (!accessTokenRef.get().isNullOrBlank()) return
        val refresh = refreshTokenRef.get() ?: vault.readRefreshToken() ?: return
        val tokens = refreshInternal(refresh)
        applyTokens(tokens, previousUsername = usernameRef.get())
    }

    private suspend fun refreshInternal(refreshToken: String): AuthTokens {
        // Deduplicate concurrent refresh with a single in-flight deferred.
        val (deferred, isOwner) = refreshMutex.withLock {
            val existing = inFlightRefresh
            if (existing != null) {
                existing to false
            } else {
                val created = CompletableDeferred<AuthTokens>()
                inFlightRefresh = created
                created to true
            }
        }
        if (!isOwner) {
            return deferred.await()
        }

        return try {
            val tokens = api.refresh(refreshToken)
            deferred.complete(tokens)
            tokens
        } catch (e: Exception) {
            deferred.completeExceptionally(e)
            throw e
        } finally {
            refreshMutex.withLock {
                if (inFlightRefresh === deferred) {
                    inFlightRefresh = null
                }
            }
        }
    }

    private suspend fun applyTokens(tokens: AuthTokens, previousUsername: String?) {
        stateMutex.withLock {
            accessTokenRef.set(tokens.accessToken)
            refreshTokenRef.set(tokens.refreshToken)
            if (previousUsername != null) {
                usernameRef.set(previousUsername)
            }
            accessTokenSink(tokens.accessToken)
            // Rotate encrypted refresh only after successful network response.
            vault.saveRefreshToken(tokens.refreshToken)
        }
    }

    private suspend fun clearLocalCredentials() {
        stateMutex.withLock {
            clearMemoryLocked()
            vault.clear()
        }
    }

    private fun clearMemory() {
        accessTokenRef.set(null)
        refreshTokenRef.set(null)
        usernameRef.set(null)
        accessTokenSink(null)
    }

    private fun clearMemoryLocked() {
        accessTokenRef.set(null)
        refreshTokenRef.set(null)
        usernameRef.set(null)
        accessTokenSink(null)
    }
}

sealed class AuthState {
    data object Unknown : AuthState()
    data object Restoring : AuthState()
    data object Authenticating : AuthState()
    data object SignedOut : AuthState()
    data class SignedIn(val username: String?) : AuthState()
}
