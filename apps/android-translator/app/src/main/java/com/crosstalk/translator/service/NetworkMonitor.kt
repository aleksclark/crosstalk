package com.crosstalk.translator.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference

/**
 * Tracks validated networks via [ConnectivityManager.NetworkCallback].
 * Distinguishes unavailable/lost vs validated so reconnect can pause correctly.
 */
class NetworkMonitor(
    context: Context,
    private val onValidatedChanged: (validated: Boolean) -> Unit,
) {
    private val appContext = context.applicationContext
    private val connectivity =
        appContext.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

    private val registered = AtomicBoolean(false)
    private val validated = AtomicBoolean(false)
    private val activeNetwork = AtomicReference<Network?>(null)

    private val callback =
        object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                activeNetwork.set(network)
                // Wait for onCapabilitiesChanged / onBlockedStatusChanged for validation.
            }

            override fun onCapabilitiesChanged(
                network: Network,
                networkCapabilities: NetworkCapabilities,
            ) {
                activeNetwork.set(network)
                val ok =
                    networkCapabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
                        networkCapabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
                setValidated(ok)
            }

            override fun onLost(network: Network) {
                if (activeNetwork.get() == network) {
                    activeNetwork.set(null)
                    setValidated(false)
                } else {
                    // Recompute from default network.
                    refreshFromSystem()
                }
            }

            override fun onUnavailable() {
                activeNetwork.set(null)
                setValidated(false)
            }
        }

    fun isValidated(): Boolean = validated.get()

    fun start() {
        if (!registered.compareAndSet(false, true)) return
        val request =
            NetworkRequest.Builder()
                .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                .build()
        try {
            connectivity.registerNetworkCallback(request, callback)
        } catch (e: RuntimeException) {
            registered.set(false)
            throw e
        }
        refreshFromSystem()
    }

    fun stop() {
        if (!registered.compareAndSet(true, false)) return
        try {
            connectivity.unregisterNetworkCallback(callback)
        } catch (_: RuntimeException) {
            // Already unregistered.
        }
        activeNetwork.set(null)
        validated.set(false)
    }

    private fun refreshFromSystem() {
        val network = connectivity.activeNetwork
        val caps = network?.let { connectivity.getNetworkCapabilities(it) }
        activeNetwork.set(network)
        val ok =
            caps != null &&
                caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
                caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
        setValidated(ok)
    }

    private fun setValidated(ok: Boolean) {
        val previous = validated.getAndSet(ok)
        if (previous != ok) {
            onValidatedChanged(ok)
        }
    }
}
