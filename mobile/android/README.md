# DownKit Android（第一阶段）

这是最小 Android 宿主，不是另写一套下载器：

1. 接收浏览器扩展发出的 `downkit://download?...`；
2. 支持在应用内粘贴 MP4/HLS 媒体地址，也可从系统分享菜单接收文本链接；
3. 使用前台 `dataSync` Service 执行下载；
4. 通过 gomobile AAR 调用仓库根目录的 Go 下载引擎；
5. 使用 Android `MediaExtractor + MediaMuxer` 尝试无损封装标准 H.264/H.265 + AAC HLS。

系统分享入口只预填链接，由用户确认后开始下载。普通网页不是媒体直链，需要浏览器扩展
先通过 `webRequest` 捕获真实 MP4/HLS 地址，再用 `downkit://` 交给应用。Android 浏览器
不能使用桌面端的 Native Messaging/Bridge 通道。YouTube 通常使用 DASH 分离音视频轨，当前
APK 未集成移动端 yt-dlp，也没有 extension 到 App 的双轨任务协议，因此暂不支持。

Android 通过 gomobile 的 `MobileToolsJSON()` 读取与桌面相同的 Tool 描述协议，但只注册移动端实际存在的 Tool。桌面 Bridge、Native Messaging Host 和 yt-dlp 不会出现在移动端 Registry。

系统封装器不是 FFmpeg 的完整替代。遇到 Android MediaExtractor 无法读取的本地 HLS、异常
TS 时间戳或特殊音轨时任务会明确失败并保留 Go 工作目录；后续把
`build/ffmpeg-android` 产物接入同一个 `PlatformMuxer` 接口即可，无需修改下载器。
FFmpeg 原生库必须随 APK/AAB 按 ABI 发布，不能使用桌面端的运行时组件下载机制。

## 构建准备

- Android Studio / SDK 35 / JDK 17
- Android NDK（gomobile 和精简 FFmpeg 构建均需要）
- `gomobile`

设置标准工具链环境变量；脚本也能从 `PATH` 发现 `javac`，并在 Android SDK 的 `ndk` 目录中选择已安装的最新 NDK：

```powershell
$env:JAVA_HOME = 'C:\path\to\jdk-17'
$env:ANDROID_SDK_ROOT = 'C:\path\to\Android\Sdk'
# 可选；同时安装多个 NDK 时用于固定版本
$env:ANDROID_NDK_HOME = 'C:\path\to\Android\Sdk\ndk\<version>'
```

```powershell
$mobileVersion = 'v0.0.0-20240326195318-268e6c3a80d1'
go install "golang.org/x/mobile/cmd/gomobile@$mobileVersion"
go install "golang.org/x/mobile/cmd/gobind@$mobileVersion"
.\mobile\android\build-android.ps1
```

脚本会依次生成 arm64 AAR 和 debug APK，并复制到
`dist/downkit-android-arm64-debug.apk`。Android 10 及以上下载完成后会通过 MediaStore
发布到公共的 `下载/DownKit` 目录，不申请宽泛的存储权限；较旧系统暂时保存在应用专属目录。

## 已知边界

- 需要 yt-dlp 从来源页面解析的 DASH/M4S 分离媒体轨仍由桌面端处理，移动端会明确拒绝。
- 当前没有任务列表，只用系统通知显示结果。
- 当前没有 FFmpeg JNI wrapper；系统封装失败时不会静默产出损坏文件。
