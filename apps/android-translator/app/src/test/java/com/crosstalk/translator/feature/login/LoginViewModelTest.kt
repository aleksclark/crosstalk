package com.crosstalk.translator.feature.login

import com.crosstalk.translator.auth.AuthRepository
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
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class LoginViewModelTest {
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
    fun emptyCredentialsShowErrorWithoutCallingApi() = runTest(dispatcher) {
        val api = FakeApi()
        val vm = LoginViewModel(repo(api), deploymentIdentity = "http://10.0.2.2:8080")
        vm.submit()
        advanceUntilIdle()
        assertEquals("Enter username and password", vm.uiState.value.errorMessage)
        assertEquals(0, api.loginCalls)
    }

    @Test
    fun invalidCredentialsSurfaceErrorAndClearPassword() = runTest(dispatcher) {
        val api = FakeApi(loginError = ApiException.Unauthorized())
        val vm = LoginViewModel(repo(api), deploymentIdentity = "https://crosstalk.local")
        vm.onUsernameChange("translator")
        vm.onPasswordChange("wrong")
        vm.submit()
        advanceUntilIdle()
        assertEquals("Invalid username or password", vm.uiState.value.errorMessage)
        assertEquals("", vm.uiState.value.password)
        assertNull(vm.uiState.value.signedInUsername)
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun successfulLoginClearsPasswordAndSignalsSignedIn() = runTest(dispatcher) {
        val api = FakeApi()
        val vm = LoginViewModel(repo(api), deploymentIdentity = "https://crosstalk.local")
        vm.onUsernameChange("translator")
        vm.onPasswordChange("good-password")
        vm.submit()
        advanceUntilIdle()
        assertEquals("translator", vm.uiState.value.signedInUsername)
        assertEquals("", vm.uiState.value.password)
        assertNull(vm.uiState.value.errorMessage)
        assertFalse(vm.uiState.value.isLoading)
    }

    @Test
    fun networkErrorShowsUnavailableMessage() = runTest(dispatcher) {
        val api = FakeApi(loginError = ApiException.Network("boom"))
        val vm = LoginViewModel(repo(api), deploymentIdentity = "https://crosstalk.local")
        vm.onUsernameChange("translator")
        vm.onPasswordChange("pw")
        vm.submit()
        advanceUntilIdle()
        assertTrue(vm.uiState.value.errorMessage!!.contains("unavailable", ignoreCase = true))
        assertEquals("", vm.uiState.value.password)
    }

    private fun repo(api: CrossTalkApi): AuthRepository {
        val vault = CredentialVault.createForTests(cipher = FakeKeystoreCipher())
        return AuthRepository(api = api, vault = vault, accessTokenSink = {})
    }

    private class FakeApi(
        private val loginError: Exception? = null,
    ) : CrossTalkApi {
        var loginCalls: Int = 0

        override suspend fun login(username: String, password: String): AuthTokens {
            loginCalls++
            loginError?.let { throw it }
            return AuthTokens(accessToken = "a", refreshToken = "r")
        }

        override suspend fun refresh(refreshToken: String): AuthTokens =
            AuthTokens(accessToken = "a", refreshToken = "r")

        override suspend fun logout(refreshToken: String) = Unit
        override suspend fun listSessions(): List<SessionSummary> = emptyList()
        override suspend fun getSession(sessionId: String): SessionSummary = error("unused")
        override suspend fun listChannels(sessionId: String): List<ChannelInfo> = emptyList()
        override suspend fun mintMediaTicket(sessionId: String, role: String): MediaTicket =
            error("unused")
    }
}
