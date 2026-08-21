package com.downkit.mobile

import android.util.Log

internal object MobileLog {
    private const val TAG = "DownKitMobile"
    private val urlPattern = Regex("""(?i)https?://[^\s\"']+""")
    private val secretPattern = Regex("""(?i)(token|signature|sig|key|auth|cookie)=([^\s&]+)""")

    fun info(
        requestId: String?,
        node: String,
        operation: String,
        status: String,
        durationMs: Long? = null,
        vararg details: Pair<String, Any?>,
    ) {
        Log.i(TAG, format(requestId, node, operation, status, durationMs, details.asIterable()))
    }

    fun warn(
        requestId: String?,
        node: String,
        operation: String,
        status: String,
        durationMs: Long? = null,
        vararg details: Pair<String, Any?>,
    ) {
        Log.w(TAG, format(requestId, node, operation, status, durationMs, details.asIterable()))
    }

    fun error(
        requestId: String?,
        node: String,
        operation: String,
        status: String,
        error: Throwable,
        durationMs: Long? = null,
        vararg details: Pair<String, Any?>,
    ) {
        val context = error.stackTrace.take(3).joinToString(" <- ") {
            "${it.className}.${it.methodName}:${it.lineNumber}"
        }
        Log.e(
            TAG,
            format(
                requestId,
                node,
                operation,
                status,
                durationMs,
                details.asIterable() + listOf(
                    "errorType" to error.javaClass.simpleName,
                    "error" to safeErrorMessage(error),
                    "exceptionContext" to context,
                ),
            ),
        )
    }

    internal fun safeErrorMessage(error: Throwable): String {
        val message = error.message?.ifBlank { error.javaClass.simpleName } ?: error.javaClass.simpleName
        return secretPattern.replace(urlPattern.replace(message, "<url>")) {
            "${it.groupValues[1]}=<redacted>"
        }
            .replace('\n', ' ')
            .replace('\r', ' ')
            .take(240)
    }

    private fun format(
        requestId: String?,
        node: String,
        operation: String,
        status: String,
        durationMs: Long?,
        details: Iterable<Pair<String, Any?>>,
    ): String = buildString {
        appendField("requestId", requestId?.takeIf { it.isNotBlank() } ?: "unknown")
        appendField("node", node)
        appendField("operation", operation)
        appendField("status", status)
        if (durationMs != null) appendField("durationMs", durationMs)
        for ((key, value) in details) appendField(key, value)
    }.trim()

    private fun StringBuilder.appendField(key: String, value: Any?) {
        if (isNotEmpty()) append(' ')
        append(key)
        append('=')
        append('"')
        append(
            value.toString()
                .replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\n", "\\n")
                .replace("\r", "\\r"),
        )
        append('"')
    }
}
