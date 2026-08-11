package com.crosstalk.translator.feature.sessions

import com.crosstalk.translator.auth.AuthRepository
import com.crosstalk.translator.auth.AuthState
import com.crosstalk.translator.auth.CredentialVault
import com.crosstalk.translator.auth.FakeKeystoreCipher
import com.crosstalk.translator.contract.ApiException
import com.crosstalk.translator.contract.AuthTokens
import com.crosstalk.translator.contract.ChannelInfo
import com.crosstalk.translator.contract.CrossTalkApi
import com.crosstalk.translator.contract.MediaTicket
import com.crosstalk.translator.contract.SessionSummary
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class SessionListViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun loadsAssignedSessionsInServerOrderWithNamePrimary() = runTest(dispatcher) {
        val api = FakeApi(
            sessions = listOf(
                SessionSummary(id = "01BBB", name = "Sunday Spanish", description = "AM"),
                SessionSummary(id = "01AAA", name = "Evening French", description = null),
            ),
        )
        val repository = repo(api)
        repository.login("maria", "pw")
        val vm = SessionListViewModel(repository)
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.sessions.size)
        assertEquals("Sunday Spanish", vm.uiState.value.sessions[0].name)
        assertEquals("Evening French", vm.uiState.value.sessions[1].name)
        assertEquals("01BBB", vm.uiState.value.sessions[0].id)
        assertEquals("maria", vm.uiState.value.username)
    }

    @Test
    fun emptyAssignmentShowsEmptyList() = runTest(dispatcher) {
        val api = FakeApi(sessions = emptyList())
        val repository = repo(api)
        repository.login("maria", "pw")
        val vm = SessionListViewModel(repository)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.sessions.isEmpty())
        assertEquals(null, vm.uiState.value.errorMessage)
    }

    @Test
    fun revokedRefreshMarksLoggedOut() = runTest(dispatcher) {
        val api = FakeApi(listError = ApiException.Unauthorized())
        val repository = repo(api)
        repository.login("maria", "pw")
        val vm = SessionListViewModel(repository)
        advanceUntilIdle()
        assertTrue(vm.uiState.value.loggedOut)
        assertTrue(vm.uiState.value.errorMessage!!.contains("expired", ignoreCase = true))
    }

    @Test
    fun logoutClearsAndSignalsLoggedOut() = runTest(dispatcher) {
        val api = FakeApi(
            sessions = listOf(SessionSummary(id = "01X", name = "One")),
        )
        val repository = repo(api)
        repository.login("maria", "pw")
        val vm = SessionListViewModel(repository)
        advanceUntilIdle()
        vm.logout()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.loggedOut)
        assertTrue(vm.uiState.value.sessions.isEmpty())
        assertTrue(repository.authState.value is AuthState.SignedOut)
    }

    private fun repo(api: CrossTalkApi): AuthRepository {
        val vault = CredentialVault.createForTests(cipher = FakeKeystoreCipher())
        return AuthRepository(api = api, vault = vault, accessTokenSink = {})
    }

    private class FakeApi(
        private val sessions: List<SessionSummary> = emptyList(),
        private val listError: Exception? = null,
    ) : CrossTalkApi {
        override suspend fun login(username: String, password: String): AuthTokens =
            AuthTokens(accessToken = "a", refreshToken = "r")

        override suspend fun refresh(refreshToken: String): AuthTokens =
            AuthTokens(accessToken = "a", refreshToken = "r")

        override suspend fun logout(refreshToken: String) = Unit

        override suspend fun listSessions(): List<SessionSummary> {
            listError?.let { throw it }
            return sessions
        }

        override suspend fun getSession(sessionId: String): SessionSummary =
            sessions.first { it.id == sessionId }

        override suspend fun listChannels(sessionId: String): List<ChannelInfo> = emptyList()
        override suspend fun mintMediaTicket(sessionId: String, role: String): MediaTicket =
            error("unused")
    }
}
