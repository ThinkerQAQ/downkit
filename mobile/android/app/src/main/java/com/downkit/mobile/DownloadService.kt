package com.downkit.mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.os.IBinder
import android.os.SystemClock
import downkit.Downkit
import org.json.JSONObject
import java.util.concurrent.Executors

class DownloadService : Service() {
    private val executor = Executors.newSingleThreadExecutor()

    override fun onCreate() {
        super.onCreate()
        MobileLog.info("service", "download-service", "create", "start")
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "媒体下载", NotificationManager.IMPORTANCE_LOW)
        )
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val task = intent?.getStringExtra(EXTRA_TASK_JSON)
        if (task.isNullOrBlank()) {
            MobileLog.warn("unknown", "download-service", "start-command", "rejected", null, "reason" to "missing-task")
            stopSelf(startId)
            return START_NOT_STICKY
        }
        val requestId = runCatching { JSONObject(task).optString("requestId") }.getOrDefault("unknown")
        val queuedAt = SystemClock.elapsedRealtime()
        MobileLog.info(requestId, "download-service", "start-command", "accepted", null, "startId" to startId)
        startForeground(NOTIFICATION_ID, notification("正在准备下载…", true))
        MobileLog.info(requestId, "download-service", "start-foreground", "ready")
        executor.execute {
            val startedAt = System.currentTimeMillis()
            val executionStartedAt = SystemClock.elapsedRealtime()
            MobileLog.info(
                requestId,
                "download-service",
                "execute-download",
                "start",
                null,
                "queueDurationMs" to executionStartedAt - queuedAt,
            )
            try {
                Downkit.downloadMobile(task, AndroidMediaMuxer())
                MobileLog.info(
                    requestId,
                    "download-service",
                    "download-core",
                    "succeeded",
                    SystemClock.elapsedRealtime() - executionStartedAt,
                )
                val message = try {
                    val publishStartedAt = SystemClock.elapsedRealtime()
                    val location = MediaPublisher(this).publishLatest(task, startedAt, requestId)
                    MobileLog.info(
                        requestId,
                        "download-service",
                        "publish-media",
                        if (location == null) "skipped" else "succeeded",
                        SystemClock.elapsedRealtime() - publishStartedAt,
                    )
                    if (location == null) "下载完成，文件保存在应用目录" else "下载完成：$location"
                } catch (error: Throwable) {
                    MobileLog.error(requestId, "download-service", "publish-media", "failed", error)
                    "下载完成，但发布到下载目录失败：${MobileLog.safeErrorMessage(error)}"
                }
                notify(message)
                MobileLog.info(requestId, "download-service", "completion-notification", "posted")
            } catch (error: Throwable) {
                MobileLog.error(
                    requestId,
                    "download-service",
                    "execute-download",
                    "failed",
                    error,
                    SystemClock.elapsedRealtime() - executionStartedAt,
                )
                notify("下载失败：${MobileLog.safeErrorMessage(error)}")
            } finally {
                stopForeground(STOP_FOREGROUND_DETACH)
                stopSelf(startId)
                MobileLog.info(
                    requestId,
                    "download-service",
                    "execute-download",
                    "stopped",
                    SystemClock.elapsedRealtime() - executionStartedAt,
                )
            }
        }
        return START_NOT_STICKY
    }

    private fun notification(text: String, ongoing: Boolean): Notification =
        Notification.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle("DownKit")
            .setContentText(text)
            .setContentIntent(
                PendingIntent.getActivity(
                    this,
                    0,
                    Intent(this, MainActivity::class.java),
                    PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
                )
            )
            .setOngoing(ongoing)
            .setAutoCancel(!ongoing)
            .build()

    private fun notify(text: String) {
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification(text, false))
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        executor.shutdownNow()
        MobileLog.info("service", "download-service", "destroy", "complete")
        super.onDestroy()
    }

    companion object {
        const val EXTRA_TASK_JSON = "task_json"
        private const val CHANNEL_ID = "downkit_download"
        private const val NOTIFICATION_ID = 4210
    }
}
