# Release 组件清单

DownKit 的桌面发布包保留独立 sidecar，不把第三方可执行文件嵌入主程序：

- `tools/ffmpeg-slim[.exe]` 随标准发布包提供；
- `yt-dlp` 是可选 Tool，缺失时由桌面端在用户点击后从官方 release 下载；
- `components.json` 记录交付方式、能力、随包文件 SHA-256 和许可证/源码信息；按需下载组件不记录本地开发目录中的同名文件。

Windows 发布目录准备好后运行：

```powershell
.\build\release\write-components.ps1
```

Linux/macOS 构建应传入相应的 `-Platform` 和 `-Architecture`，并保持同样的目录布局。
Android/iOS 不使用该下载机制；原生库必须随 APK/AAB 或应用包发布。

创建 GitHub Release 资产：

```powershell
.\build\release\package.ps1 -FFmpegSourceArchive .\.build\FFmpeg-n8.1.2.tar.gz
```

脚本会重新编译五个桌面目标，生成一个 Windows ZIP、四个 Linux/macOS TAR.GZ、FFmpeg 对应源码包和 `SHA256SUMS.txt`。
