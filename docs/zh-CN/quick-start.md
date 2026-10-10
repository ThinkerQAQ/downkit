# 快速入门

1. 从 [GitHub Releases](https://github.com/ThinkerQAQ/downkit/releases) 下载发行包并解压，保留 Bridge、`extension/` 和 `tools/` 目录。
2. Windows 运行 `bin\downkitdev.exe install`；Linux/macOS 运行 `./bin/downkitdev install`。Linux/macOS 需要自行安装 FFmpeg。
3. 在 `chrome://extensions` 或 `edge://extensions` 加载解压后的扩展。
4. 打开视频网页并播放，通过 DownKit 的「资源嗅探」发起下载。

扩展通过 Native Messaging 按需启动 Bridge。下一步：[首次下载教程](tutorial/first-download.md)。
