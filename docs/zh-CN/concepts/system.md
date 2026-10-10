# 系统组成

DownKit 包括浏览器扩展、桌面 Go Bridge 和独立 Android 应用。扩展管理媒体检测和浏览器会话；Bridge 管理桌面下载任务、状态和依赖工具；Android 接收分享或粘贴的媒体 URL。

Popup、Side Panel 和 Web UI 复用页面逻辑。Web UI 位于 `http://127.0.0.1:17891/console/`，授权操作由扩展中继，网页本身不持有 Native Messaging Token。
