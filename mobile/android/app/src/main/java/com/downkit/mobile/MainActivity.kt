package com.downkit.mobile

import android.Manifest
import android.app.Activity
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.text.InputType
import android.view.Gravity
import android.view.inputmethod.EditorInfo
import android.widget.Button
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import downkit.Downkit
import org.json.JSONObject
import java.util.UUID

class MainActivity : Activity() {
    private lateinit var status: TextView
    private lateinit var urlInput: EditText

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        MobileLog.info("app", "activity", "create", "start")
        setContentView(buildContentView())
        requestNotificationPermission()
        accept(intent)
        MobileLog.info("app", "activity", "create", "ready")
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        accept(intent)
    }

    private fun buildContentView(): ScrollView {
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
            setPadding(24.dp, 32.dp, 24.dp, 32.dp)
        }
        content.addView(ImageView(this).apply {
            setImageResource(R.drawable.ic_downkit_logo)
            contentDescription = "DownKit"
        }, LinearLayout.LayoutParams(88.dp, 88.dp))
        content.addView(TextView(this).apply {
            text = "DownKit"
            textSize = 28f
            gravity = Gravity.CENTER
            setPadding(0, 8.dp, 0, 20.dp)
        }, matchWidth())
        urlInput = EditText(this).apply {
            hint = "粘贴 MP4 或 HLS（m3u8）媒体地址"
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
            isSingleLine = true
            imeOptions = EditorInfo.IME_ACTION_GO
            setOnEditorActionListener { _, actionId, _ ->
                if (actionId == EditorInfo.IME_ACTION_GO) {
                    submitManualInput()
                    true
                } else {
                    false
                }
            }
        }
        content.addView(urlInput, matchWidth())

        val actions = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER
        }
        actions.addView(Button(this).apply {
            text = "粘贴"
            setOnClickListener { pasteFromClipboard() }
        }, weightedWidth())
        actions.addView(Button(this).apply {
            text = "开始下载"
            setOnClickListener { submitManualInput() }
        }, weightedWidth())
        content.addView(actions, matchWidth().apply { topMargin = 12.dp })

        content.addView(TextView(this).apply {
            text = "也可以在浏览器或其他应用中点“分享”，然后选择 DownKit。\n\n当前移动端直接支持 MP4/HLS 媒体地址；普通网页需要扩展先捕获媒体地址。YouTube 常用分离音视频轨，当前移动端尚未支持。"
            textSize = 14f
            setPadding(0, 20.dp, 0, 12.dp)
        }, matchWidth())
        status = TextView(this).apply {
            textSize = 15f
            text = "已就绪。完成的视频会保存到：下载/DownKit\n\n${mobileToolSummary()}"
        }
        content.addView(status, matchWidth())
        return ScrollView(this).apply { addView(content) }
    }

    private fun accept(intent: Intent?) {
        if (intent == null) return
        if (intent.action == Intent.ACTION_SEND && intent.type?.startsWith("text/") == true) {
            acceptSharedText(intent)
            return
        }
        val uri = intent.data ?: return
        if (uri.scheme != "downkit" || uri.host != "download") return
        val source = normalizeHttpUrl(uri.getQueryParameter("url").orEmpty())
        if (source == null) {
            rejectInput("deep-link", "拒绝了无效的媒体 URL。")
            return
        }
        dispatchDownload(
            source = source,
            title = uri.getQueryParameter("title").orEmpty(),
            referer = uri.getQueryParameter("referer").orEmpty(),
            origin = uri.getQueryParameter("origin").orEmpty(),
            userAgent = uri.getQueryParameter("ua").orEmpty(),
            quality = uri.getQueryParameter("quality") ?: "best",
            ingress = "deep-link",
        )
    }

    private fun acceptSharedText(intent: Intent) {
        val shared = intent.getCharSequenceExtra(Intent.EXTRA_TEXT)?.toString().orEmpty()
        val source = extractHttpUrl(shared)
        if (source == null) {
            rejectInput("share", "分享内容中没有有效的 HTTP/HTTPS 地址。")
            return
        }
        urlInput.setText(source)
        urlInput.setSelection(source.length)
        status.text = "已接收分享链接。请确认它是 MP4/HLS 媒体地址，然后点“开始下载”。"
        MobileLog.info("manual", "activity", "accept-share", "ready", null, "scheme" to source.substringBefore(':'))
    }

    private fun pasteFromClipboard() {
        val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val text = clipboard.primaryClip?.takeIf { it.itemCount > 0 }
            ?.getItemAt(0)?.coerceToText(this)?.toString().orEmpty()
        val source = extractHttpUrl(text)
        if (source == null) {
            rejectInput("clipboard", "剪贴板中没有有效的 HTTP/HTTPS 地址。")
            return
        }
        urlInput.setText(source)
        urlInput.setSelection(source.length)
        status.text = "已粘贴媒体地址，请确认后开始下载。"
    }

    private fun submitManualInput() {
        val source = extractHttpUrl(urlInput.text?.toString().orEmpty())
        if (source == null) {
            urlInput.error = "请输入有效的 HTTP/HTTPS 媒体地址"
            rejectInput("manual", "请输入有效的 MP4/HLS 媒体地址。")
            return
        }
        urlInput.error = null
        dispatchDownload(source = source, ingress = "manual")
    }

    private fun dispatchDownload(
        source: String,
        title: String = "",
        referer: String = "",
        origin: String = "",
        userAgent: String = "",
        quality: String = "best",
        ingress: String,
    ) {
        val requestId = UUID.randomUUID().toString()
        val outputDir = getExternalFilesDir(Environment.DIRECTORY_MOVIES)?.absolutePath
            ?: filesDir.absolutePath
        val task = JSONObject().apply {
            put("requestId", requestId)
            put("url", source)
            put("title", title)
            put("referer", referer)
            put("origin", origin)
            put("ua", userAgent)
            put("quality", quality)
            put("outputDir", outputDir)
            put("concurrent", 8)
        }.toString()
        val service = Intent(this, DownloadService::class.java)
            .putExtra(DownloadService.EXTRA_TASK_JSON, task)
        MobileLog.info(
            requestId,
            "activity",
            "dispatch-download",
            "start",
            null,
            "ingress" to ingress,
            "scheme" to source.substringBefore(':'),
        )
        startForegroundService(service)
        status.text = "任务已交给后台下载服务。完成后会保存到：下载/DownKit"
    }

    private fun rejectInput(ingress: String, message: String) {
        MobileLog.warn(
            "manual",
            "activity",
            "accept-input",
            "rejected",
            null,
            "ingress" to ingress,
            "reason" to "invalid-url",
        )
        status.text = message
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 100)
            MobileLog.info("app", "activity", "request-notification-permission", "requested")
        }
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 100) {
            val granted = grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED
            MobileLog.info(
                "app",
                "activity",
                "request-notification-permission",
                if (granted) "granted" else "denied",
            )
        }
    }

    private fun mobileToolSummary(): String = runCatching {
        val tools = JSONObject(Downkit.mobileToolsJSON()).getJSONArray("tools")
        buildString {
            appendLine("组件状态")
            for (index in 0 until tools.length()) {
                val tool = tools.getJSONObject(index)
                val health = tool.getJSONObject("health")
                val marker = if (health.optBoolean("ok")) "●" else "○"
                append(marker)
                append(' ')
                append(tool.optString("displayName", tool.optString("name")))
                append(" · ")
                append(health.optString("summary", health.optString("status")))
                if (index < tools.length() - 1) appendLine()
            }
        }
    }.getOrElse { error ->
        MobileLog.error("app", "activity", "load-tool-summary", "failed", error)
        "组件状态暂不可用"
    }

    private fun matchWidth() = LinearLayout.LayoutParams(
        LinearLayout.LayoutParams.MATCH_PARENT,
        LinearLayout.LayoutParams.WRAP_CONTENT,
    )

    private fun weightedWidth() = LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
    private val Int.dp: Int get() = (this * resources.displayMetrics.density).toInt()
}
