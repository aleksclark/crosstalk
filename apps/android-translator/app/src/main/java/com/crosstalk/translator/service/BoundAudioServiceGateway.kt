package com.crosstalk.translator.service

import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.os.IBinder
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.util.concurrent.atomic.AtomicReference

/**
 * Production [AudioServiceGateway]: starts/binds [TranslatorAudioService] and
 * mirrors its StateFlow. Join starts the FGS from a visible activity context.
 */
class BoundAudioServiceGateway(
    context: Context,
) : AudioServiceGateway {
    private val appContext = context.applicationContext

    private val _state = MutableStateFlow(ServiceState.Idle)
    override val state: StateFlow<ServiceState> = _state.asStateFlow()

    private val binderRef = AtomicReference<TranslatorAudioService.LocalBinder?>(null)
    private var bound = false
    private var pendingJoin: ServiceCommand.Join? = null
    private var pendingMute: Boolean? = null
    private var pendingStop = false

    private val connection =
        object : ServiceConnection {
            override fun onServiceConnected(name: ComponentName?, service: IBinder?) {
                val binder = service as? TranslatorAudioService.LocalBinder ?: return
                binderRef.set(binder)
                bound = true
                // Collect is owned by the binder's StateFlow — poll snapshot + observe via callback.
                binder.setStateListener { _state.value = it }
                _state.value = binder.state.value

                val stopNow = pendingStop
                pendingStop = false
                pendingJoin?.let {
                    pendingJoin = null
                    if (!stopNow) {
                        binder.dispatch(it)
                    }
                }
                pendingMute?.let {
                    pendingMute = null
                    if (!stopNow) {
                        binder.dispatch(ServiceCommand.SetMuted(it))
                    }
                }
                if (stopNow) {
                    binder.dispatch(ServiceCommand.Stop)
                }
            }

            override fun onServiceDisconnected(name: ComponentName?) {
                binderRef.set(null)
                bound = false
            }
        }

    override fun join(
        sessionId: String,
        sessionName: String,
        feedName: String,
        broadcastName: String,
        feedIds: List<String>,
        broadcastIds: List<String>,
    ) {
        val cmd =
            ServiceCommand.Join(
                sessionId = sessionId,
                sessionName = sessionName,
                feedName = feedName,
                broadcastName = broadcastName,
                feedIds = feedIds,
                broadcastIds = broadcastIds,
            )
        val binder = binderRef.get()
        if (binder != null) {
            ensureStarted(cmd)
            binder.dispatch(cmd)
        } else {
            pendingJoin = cmd
            ensureStarted(cmd)
            bind()
        }
    }

    override fun setMuted(muted: Boolean) {
        val binder = binderRef.get()
        if (binder != null) {
            binder.dispatch(ServiceCommand.SetMuted(muted))
        } else {
            pendingMute = muted
            bind()
        }
    }

    override fun stop() {
        val binder = binderRef.get()
        if (binder != null) {
            binder.dispatch(ServiceCommand.Stop)
        } else {
            pendingStop = true
            // Still deliver stop via startService action so notification Stop works offline of bind.
            val intent =
                Intent(appContext, TranslatorAudioService::class.java).apply {
                    action = TranslatorAudioService.ACTION_STOP
                }
            try {
                appContext.startService(intent)
            } catch (_: Exception) {
                // Service not running.
            }
        }
    }

    override fun bind() {
        if (bound) return
        val intent = Intent(appContext, TranslatorAudioService::class.java)
        appContext.bindService(intent, connection, Context.BIND_AUTO_CREATE)
    }

    override fun unbind() {
        if (!bound) return
        try {
            binderRef.get()?.setStateListener(null)
            appContext.unbindService(connection)
        } catch (_: Exception) {
            // Already unbound.
        }
        binderRef.set(null)
        bound = false
    }

    private fun ensureStarted(join: ServiceCommand.Join) {
        val intent =
            Intent(appContext, TranslatorAudioService::class.java).apply {
                action = TranslatorAudioService.ACTION_JOIN
                putExtra(TranslatorAudioService.EXTRA_SESSION_ID, join.sessionId)
                putExtra(TranslatorAudioService.EXTRA_SESSION_NAME, join.sessionName)
                putExtra(TranslatorAudioService.EXTRA_FEED_NAME, join.feedName)
                putExtra(TranslatorAudioService.EXTRA_BROADCAST_NAME, join.broadcastName)
                putStringArrayListExtra(
                    TranslatorAudioService.EXTRA_FEED_IDS,
                    ArrayList(join.feedIds),
                )
                putStringArrayListExtra(
                    TranslatorAudioService.EXTRA_BROADCAST_IDS,
                    ArrayList(join.broadcastIds),
                )
            }
        ContextCompat.startForegroundService(appContext, intent)
    }
}
