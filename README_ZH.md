# DownKit

[English](README.md) · [中文文档](docs/zh-CN/index.md) · [Documentation](docs/index.md)

DownKit 是一个通过 Chrome 或 Edge 扩展发现网页中的 M3U8、DASH 和 MP4 媒体，并交给本机 Bridge 下载和封装的跨平台工具。

## 字幕能力

Windows 版支持在下载媒体的同时生成独立字幕文件：

- 优先下载网站提供的人工字幕或自动字幕；
- 原站没有所需字幕时，可使用随发行包提供的 whisper.cpp 客户端在本机识别语音；
- 可使用随发行包提供的 llama.cpp 和 TranslateGemma 在本机生成译文或双语 SRT；
- 音频提取、语音识别、模型加载、逐条翻译和字幕生成均显示独立阶段与真实进度。

Whisper 和 TranslateGemma 模型权重不包含在发行包内。请在扩展的“设置 → 下载能力”中选择模型、阅读相应许可并按需下载；下载完成后无需使用命令行。Linux 和 macOS 发行包目前不携带本地 ASR/翻译 sidecar。

## 界面预览

![DownKit v1.2.0 资源嗅探初始界面](docs/images/first-run-detect-en.png)

![DownKit v1.2.0 设置导航](docs/images/first-run-settings-en.png)

以上为当前 v1.2.0 在隔离 Chromium 中的真实英文界面，尚未连接 Bridge 或开始下载，**不是下载成功截图**。新用户请先看[带截图的首次使用教程](docs/zh-CN/tutorial/first-run-on-new-machine.md)，连接异常看[故障排查](docs/zh-CN/how-to/troubleshoot.md)。

## 安装与使用

Windows、Linux 和 macOS 的整体流程相同：加载浏览器扩展、注册本机 Bridge，然后由浏览器按需自动启动 Bridge；无需手动常驻运行 Bridge。

### Windows

1. 从 [GitHub Releases](https://github.com/ThinkerQAQ/downkit/releases) 下载 Windows 发行包并解压到固定目录，不要单独移动 Bridge、`extension` 或 `tools` 目录。
2. 在 PowerShell/终端运行 `./bin/downkitdev.exe install`。Go 安装器注册 Native Messaging Host，支持 Chrome 和 Edge。
3. 打开 `edge://extensions` 或 `chrome://extensions`，启用“开发人员模式”，选择“加载解压缩的扩展”，然后选择发行包中的 `extension` 目录。
4. 打开包含媒体的网页并开始播放，点击 DownKit 扩展图标，在检测到的资源中选择需要的项目并开始下载。
5. 如果移动了解压目录或替换了 Bridge，请重新运行 `./bin/downkitdev.exe install`。需要卸载时，运行 `./bin/downkitdev.exe uninstall`，再从浏览器中移除扩展。

### Linux 和 macOS

1. 从 [GitHub Releases](https://github.com/ThinkerQAQ/downkit/releases) 下载与系统和 CPU 架构匹配的发行包并解压到固定目录。
2. Linux/macOS 在解压目录运行 `./bin/downkitdev install`。Go 维护程序按照运行平台注册 Chrome、Chromium 和 Edge。
3. Linux/macOS 发布包不携带 FFmpeg，请先通过系统包管理器安装 `ffmpeg` 并确保它在 `PATH` 中，以便完成媒体合并和封装。
4. 打开 `edge://extensions` 或 `chrome://extensions`，启用“开发人员模式”，选择“加载解压缩的扩展”，然后选择发行包中的 `extension` 目录。
5. 打开包含媒体的网页并开始播放，点击 DownKit 扩展图标，在检测到的资源中选择需要的项目并开始下载。

如果移动了解压目录，请重新运行 Go 安装程序。卸载时运行 `./bin/downkitdev uninstall`，再从浏览器中移除扩展。

## 许可证

DownKit 自有代码和文档采用 [Apache License 2.0](LICENSE)；随项目分发的第三方组件仍遵循各自的许可证，详情见 [NOTICE](NOTICE) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 开发与构建

项目使用 `tools/downkitdev/` 原生 Go 维护程序，不依赖额外的开发网关。进入仓库根目录后运行 `go -C tools/downkitdev run . verify` 和 `go -C tools/downkitdev run . build`；发行打包、部署及平台依赖参阅[工程维护指南](docs/zh-CN/architecture/maintenance.md)。Agent 规范见 [AGENTS.md](AGENTS.md)。

## 本地配置与数据

Windows 手动部署推荐把二进制放在根目录 `bin/`，把浏览器扩展放在 `extension/`，把 FFmpeg、Whisper 和 Llama 等随包组件放在 `tools/`。本地状态放在与 `bin` 同级的 `Data/`。

- `Data/config.toml`：下载、代理、工具、界面、资源检测和日志配置。
- `Data/jobs.json`：任务状态快照；`Data/bridge.json`：Bridge 运行时状态。
- `Data/models/`、`Data/tools/`、`Data/logs/`、`Data/work/`：模型、按需下载工具、日志和下载工作文件。
- `Data/native-host/`：浏览器要求的 Native Messaging JSON 注册清单。

原来已经选择的下载目录会保留。安装后，在 `edge://extensions` 或 `chrome://extensions` 中加载或重新加载新 `extension/`。修改 TOML 文件后需要重启 Bridge，界面保存的配置则立即生效。
