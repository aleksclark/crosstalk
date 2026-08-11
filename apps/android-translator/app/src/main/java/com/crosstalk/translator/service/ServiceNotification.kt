package com.crosstalk.translator.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import androidx.core.app.NotificationCompat
import com.crosstalk.translator.MainActivity
import com.crosstalk.translator.R

/**
 * Low-importance ongoing notification for the live translation foreground service.
 * Fixed ID/channel; Mute/Unmute + Stop via immutable PendingIntents.
 */
class ServiceNotification(
    context: Context,
) {
    private val appContext = context.applicationContext
    private val manager =
        appContext.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    fun ensureChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val existing = manager.getNotificationChannel(CHANNEL_ID)
        if (existing != null) return
        val channel =
            NotificationChannel(
                CHANNEL_ID,
                appContext.getString(R.string.notification_channel_name),
                NotificationManager.IMPORTANCE_LOW,
            ).apply {
                description = appContext.getString(R.string.notification_channel_description)
                setShowBadge(false)
                enableVibration(false)
                setSound(null, null)
            }
        manager.createNotificationChannel(channel)
    }

    fun build(state: ServiceState): Notification {
        ensureChannel()
        val title =
            state.sessionName?.takeIf { it.isNotBlank() }
                ?: appContext.getString(R.string.notification_default_title)
        val listening =
            state.feedName?.takeIf { it.isNotBlank() }
                ?: appContext.getString(R.string.notification_unknown_channel)
        val speaking =
            state.broadcastName?.takeIf { it.isNotBlank() }
                ?: appContext.getString(R.string.notification_unknown_channel)
        val body =
            appContext.getString(
                R.string.notification_body,
                listening,
                speaking,
                state.notificationStatusLabel,
            )

        val contentIntent = contentPendingIntent(state.sessionId)
        val builder =
            NotificationCompat.Builder(appContext, CHANNEL_ID)
                .setSmallIcon(android.R.drawable.ic_btn_speak_now)
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(NotificationCompat.BigTextStyle().bigText(body))
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setCategory(NotificationCompat.CATEGORY_SERVICE)
                .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
                .setContentIntent(contentIntent)
                .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)

        if (state.userRequestedLive || state.phase.isActionable()) {
            if (state.micMuted) {
                builder.addAction(
                    0,
                    appContext.getString(R.string.notification_action_unmute),
                    actionPendingIntent(TranslatorAudioService.ACTION_UNMUTE),
                )
            } else {
                builder.addAction(
                    0,
                    appContext.getString(R.string.notification_action_mute),
                    actionPendingIntent(TranslatorAudioService.ACTION_MUTE),
                )
            }
            builder.addAction(
                0,
                appContext.getString(R.string.notification_action_stop),
                actionPendingIntent(TranslatorAudioService.ACTION_STOP),
            )
        }

        return builder.build()
    }

    fun notify(state: ServiceState) {
        manager.notify(NOTIFICATION_ID, build(state))
    }

    fun cancel() {
        manager.cancel(NOTIFICATION_ID)
    }

    private fun contentPendingIntent(sessionId: String?): PendingIntent {
        val intent =
            Intent(appContext, MainActivity::class.java).apply {
                flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
                action = Intent.ACTION_VIEW
                // Open live route without credentials — session id only.
                if (!sessionId.isNullOrBlank()) {
                    putExtra(EXTRA_OPEN_SESSION_ID, sessionId)
                    putExtra(EXTRA_OPEN_LIVE, true)
                }
            }
        return PendingIntent.getActivity(
            appContext,
            REQUEST_CONTENT,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun actionPendingIntent(action: String): PendingIntent {
        val intent =
            Intent(appContext, TranslatorAudioService::class.java).apply {
                this.action = action
            }
        val requestCode =
            when (action) {
                TranslatorAudioService.ACTION_MUTE -> REQUEST_MUTE
                TranslatorAudioService.ACTION_UNMUTE -> REQUEST_UNMUTE
                TranslatorAudioService.ACTION_STOP -> REQUEST_STOP
                else -> REQUEST_STOP
            }
        return PendingIntent.getService(
            appContext,
            requestCode,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun ServicePhase.isActionable(): Boolean =
        when (this) {
            ServicePhase.Preparing,
            ServicePhase.Minting,
            ServicePhase.Signaling,
            ServicePhase.IceChecking,
            ServicePhase.Connected,
            ServicePhase.Muted,
            ServicePhase.ReconnectScheduled,
            ServicePhase.WaitingForNetwork,
            -> true
            else -> false
        }

    companion object {
        const val CHANNEL_ID: String = "live_translation"
        const val NOTIFICATION_ID: Int = 0xC705 // fixed
        const val EXTRA_OPEN_SESSION_ID: String = "com.crosstalk.translator.OPEN_SESSION_ID"
        const val EXTRA_OPEN_LIVE: String = "com.crosstalk.translator.OPEN_LIVE"

        private const val REQUEST_CONTENT = 1001
        private const val REQUEST_MUTE = 1002
        private const val REQUEST_UNMUTE = 1003
        private const val REQUEST_STOP = 1004
    }
}
