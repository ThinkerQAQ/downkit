# FFmpeg Slim sidecar

这个构建只负责 DownKit 的无损封装，以及当前 yt-dlp 的音视频合并。它不包含转码编码器、滤镜集合、采集设备、网络协议、硬件加速和 `ffprobe`/`ffplay`。

Windows 构建：

```powershell
.\build\ffmpeg-slim\build-windows.ps1
```

只有构建机器直连失败时才传入代理，例如 `-NetworkProxy http://127.0.0.1:11111`。该参数只影响源码和构建依赖下载，不会写入最终程序。

默认输出为 `dist\tools\ffmpeg-slim.exe`。DownKit 会按以下顺序发现 FFmpeg：

1. 程序旁的 `tools/ffmpeg-slim`；
2. 程序旁的 `tools/ffmpeg`；
3. 程序旁的 `ffmpeg-slim`；
4. 程序旁的 `ffmpeg`；
5. 系统 PATH 和现有 Windows 兼容路径。

Linux/macOS 可以准备 FFmpeg 源码和本机编译工具后直接运行：

```sh
./build/ffmpeg-slim/build-native.sh /path/to/FFmpeg .build/ffmpeg-slim dist/tools
```

不要把这个文件当作通用 FFmpeg。若未来 yt-dlp 出现新的合并格式，再根据真实失败补一个 demuxer、muxer、parser 或 bitstream filter。

发布二进制时应同时包含 FFmpeg 源码地址、实际构建配置以及 LGPL 许可证文本；当前 Windows 构建不启用 GPL 组件。
