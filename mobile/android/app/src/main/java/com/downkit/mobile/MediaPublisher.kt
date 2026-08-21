package com.downkit.mobile

import android.content.ContentValues
import android.content.Context
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.os.SystemClock
import org.json.JSONObject
import java.io.File

class MediaPublisher(private val context: Context) {
    fun publishLatest(taskJson: String, startedAt: Long, requestId: String): String? {
        val operationStartedAt = SystemClock.elapsedRealtime()
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            MobileLog.warn(requestId, "media-publisher", "publish", "unsupported", null, "sdk" to Build.VERSION.SDK_INT)
            return null
        }
        val outputDir = JSONObject(taskJson).optString("outputDir")
        val source = File(outputDir).listFiles()
            ?.asSequence()
            ?.filter { it.isFile && it.extension.equals("mp4", ignoreCase = true) }
            ?.filter { it.lastModified() >= startedAt - 2_000 }
            ?.maxByOrNull { it.lastModified() }
            ?: run {
                MobileLog.warn(requestId, "media-publisher", "select-output", "not-found")
                return null
            }

        MobileLog.info(
            requestId,
            "media-publisher",
            "select-output",
            "selected",
            null,
            "fileName" to source.name,
            "bytes" to source.length(),
        )

        val values = ContentValues().apply {
            put(MediaStore.MediaColumns.DISPLAY_NAME, source.name)
            put(MediaStore.MediaColumns.MIME_TYPE, "video/mp4")
            put(
                MediaStore.MediaColumns.RELATIVE_PATH,
                "${Environment.DIRECTORY_DOWNLOADS}/DownKit"
            )
            put(MediaStore.MediaColumns.IS_PENDING, 1)
        }
        val resolver = context.contentResolver
        MobileLog.info(requestId, "media-publisher", "insert-media-store", "start")
        val uri = resolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)
            ?: error("MediaStore 返回空 URI")
        try {
            var copiedBytes = 0L
            resolver.openOutputStream(uri, "w")?.use { output ->
                source.inputStream().use { input -> copiedBytes = input.copyTo(output) }
            } ?: error("无法打开系统下载目录")
            MobileLog.info(
                requestId,
                "media-publisher",
                "copy-to-media-store",
                "succeeded",
                null,
                "bytes" to copiedBytes,
            )
            values.clear()
            values.put(MediaStore.MediaColumns.IS_PENDING, 0)
            val updated = resolver.update(uri, values, null, null)
            if (updated != 1) error("MediaStore 完成发布时更新了 $updated 条记录")
            if (!source.delete()) {
                MobileLog.warn(requestId, "media-publisher", "delete-source", "failed", null, "fileName" to source.name)
            }
            MobileLog.info(
                requestId,
                "media-publisher",
                "publish",
                "succeeded",
                SystemClock.elapsedRealtime() - operationStartedAt,
                "fileName" to source.name,
            )
            return "下载/DownKit/${source.name}"
        } catch (error: Throwable) {
            val deleted = resolver.delete(uri, null, null)
            MobileLog.error(
                requestId,
                "media-publisher",
                "publish",
                "failed",
                error,
                SystemClock.elapsedRealtime() - operationStartedAt,
                "cleanupRows" to deleted,
            )
            throw error
        }
    }
}
