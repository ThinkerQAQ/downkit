# Release 组件清单

DownKit 的桌面发布包保留独立 sidecar，不把第三方可执行文件嵌入主程序：

- `tools/ffmpeg-slim[.exe]` 随标准发布包提供；
- Windows 包在 `tools/whisper/` 中提供官方 CPU 版 `whisper-cli.exe` 及其运行时 DLL；模型文件不随包提供；
- Windows 包在 `tools/llama/` 中提供官方 CPU 版 `llama-server.exe` 及其运行时 DLL；翻译模型不随包提供；
- `yt-dlp` 是可选 Tool，缺失时由桌面端在用户点击后从官方 release 下载；
- `components.json` 记录交付方式、能力、随包文件 SHA-256 和许可证/源码信息；按需下载组件不记录本地开发目录中的同名文件。

Windows 组件需要准备好后才能创建正式发行包：

```powershell
go -C tools/downkitdev run . tool-install whisper
go -C tools/downkitdev run . tool-install llama
go -C tools/downkitdev run . components --root .devtool/out/release-stage/downkit-v1.2.1-windows-amd64 --os windows --arch amd64
```

Linux/macOS 由 Go 维护工具自动选择平台与架构，并保持同样的目录布局。
Android/iOS 不使用该下载机制；原生库必须随 APK/AAB 或应用包发布。

创建 GitHub Release 资产：

```powershell
# 首先将官方 FFmpeg-n8.1.2-source.tar.gz 下载到
# .devtool/cache/source/FFmpeg-n8.1.2-source.tar.gz
# 并验证仓库记录的固定 SHA256
devtool package
```

Go 维护程序会重新编译五个桌面目标，生成一个 Windows ZIP、四个 Linux/macOS TAR.GZ、FFmpeg 对应源码包和 `SHA256SUMS.txt`。
