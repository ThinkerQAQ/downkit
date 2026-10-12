# Go 工程工具与维护流程

DownKit 使用 `tools/downkitdev/` 中的原生 Go 维护程序统一执行构建、验证、发行打包、安装与平台工具集成。项目无需额外的开发网关或外部 Project SDK。

## 目录职责

| 目录 | 职责 |
| --- | --- |
| `bridge/` | Go 下载引擎、本地鉴权 API、Native Messaging 和 Android Go 绑定 |
| `extension/` | Manifest V3、Popup、Side Panel 和 Web UI |
| `mobile/android/` | Kotlin 应用、官方 Gradle Wrapper |
| `tools/downkitdev/` | 构建、验证、打包、部署、组件管理和 Android/FFmpeg 编排 |
| `docs/`、`docs/zh-CN/` | 英文与中文对照文档 |
| `.build/out/`、`.build/artifacts/`、`.build/cache/` | Git 忽略的构建产物与缓存 |
| 安装目录下的 `bin/`、`extension/`、`tools/`、`Data/` | 程序文件与持久化用户数据（部署不得清理 `Data/`） |

`bridge/` 仍是独立 Go 模块，避免破坏现有导入与 gomobile 绑定路径。

## 原生工程命令

在仓库根目录运行：

```sh
go -C tools/downkitdev run . verify
go -C tools/downkitdev run . build
go -C tools/downkitdev run . build --os windows --arch amd64
go -C tools/downkitdev run . package
go -C tools/downkitdev run . components --root /path/to/installed/downkit
go -C tools/downkitdev run . tool-install whisper
go -C tools/downkitdev run . tool-install llama
go -C tools/downkitdev run . android-bind
go -C tools/downkitdev run . android-build
go -C tools/downkitdev run . ffmpeg-native --source /path/to/FFmpeg
go -C tools/downkitdev run . ffmpeg-android --source /path/to/FFmpeg
go -C tools/downkitdev run . icons
```

`verify` 检查两个 Go 模块、JavaScript 语法和浏览器扩展测试，不代表真实浏览器、Windows 安装或 Android 真机验收。普通构建产物在 `.build/out/`，发行包在 `.build/artifacts/release/v<version>/`，Android 产物在 `.build/artifacts/android/`。

## 原生 LSP 与代码分析

- **Go：** 通过 LSP 客户端运行 `gopls serve`；`bridge/` 和 `tools/downkitdev/` 是两个独立 Go 模块。优先使用符号、定义、引用和诊断能力。
- **浏览器代码：** 有语言服务器时使用编辑器提供的 JS/HTML/CSS 导航与诊断；没有时使用 `rg`、源码检查、`node --check` 和现有测试。
- **Markdown：** 通过 LSP 客户端运行 `marksman server`，查看标题、定义与链接引用。完整审查还要逐节阅读正文和表格。

LSP 服务器不是直接执行一次就能查询的命令，也无需为了它们额外实现一套网关。

## 发布与部署

`package` 生成 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 发行包、SHA256 校验文件和 `components.json`。Windows 正式包要求 `dist/tools/` 中包含具备合法许可的 FFmpeg、Whisper、Llama 组件。缺少组件必须报错；`--allow-missing-tools` 仅允许开发用途的不完整包，不应用于正式发布。

从 WSL 部署 Windows 版之前，先停止 Bridge，再执行：

```sh
go -C tools/downkitdev run . deploy --os windows --arch amd64 --target /mnt/c/software/Coding/downkit
```

Windows 使用 `bin/downkitdev.exe install` 注册 Native Messaging，Linux/macOS 使用 `./bin/downkitdev install`；解除注册则执行 `uninstall`。安装和部署不得覆盖 `Data/`、已有下载文件和配置。

Android 保留官方 Gradle Wrapper、gomobile、SDK/NDK 和 JDK；FFmpeg 继续使用上游 configure/make 以及需要的平台工具链。没有执行成功的目标构建，不应声明已完成验收。
