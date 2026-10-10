# DEVTool 与 Go 工程管理

DownKit 参照 BlogCTL 的工程方式：仓库根目录使用 `.devtool.toml`，`devcontrol/` 只管理 DEVTool 扩展入口；实际构建、安装、打包和依赖维护统一由 `tools/downkitdev/` 的 Go 命令执行。

## 目录职责

| 目录 | 职责 |
| --- | --- |
| `bridge/` | Go 下载引擎、Native Messaging、桌面控制接口和 Android Go 绑定 |
| `extension/` | 浏览器扩展、弹窗、侧边栏与 Web UI |
| `mobile/android/` | Kotlin 宿主、官方 Gradle Wrapper |
| `tools/downkitdev/` | Go 维护程序：构建、验证、安装、发行、组件及 Android/FFmpeg |
| `devcontrol/` | DEVTool Project Extension，调用唯一的 Go 维护实现 |
| `docs/` 与 `docs/zh-CN/` | 英文和中文对照文档 |
| `.devtool/out/`、`.devtool/artifacts/` | 本地构建和发行结果 |
| 安装目录下的 `bin/`、`extension/`、`tools/`、`Data/` | 二进制与静态资源、运行时持久化数据 |

`bridge/` 保留现有 Go Module 路径。为模仿 BlogCTL 而搬迁模块，会影响现有 Go 移动绑定和运行时路径，收益不足，因此保持当前职责边界。

## DEVTool 开发入口

```sh
devtool config validate
devtool project inspect --json
devtool code doctor
devtool verify
devtool code verify
devtool build
devtool package
```

上述构建/验证/打包流程都由 `devcontrol/` 委派给同一个 Go 维护程序。DEVTool 还注册组件清单/安装、Android 构建、FFmpeg 构建、图标生成、安装与部署操作。可运行 `devtool project inspect --json` 查看完整能力。

## Go 维护程序

在源码根目录执行：

```sh
go -C tools/downkitdev run . verify
go -C tools/downkitdev run . build --os windows --arch amd64
go -C tools/downkitdev run . package
go -C tools/downkitdev run . tool-install whisper
go -C tools/downkitdev run . tool-install llama
go -C tools/downkitdev run . android-build
go -C tools/downkitdev run . ffmpeg-native --source /path/to/FFmpeg
go -C tools/downkitdev run . ffmpeg-android --source /path/to/FFmpeg
go -C tools/downkitdev run . icons
```

从 WSL 部署到 Windows：先停止正在运行的 Bridge，然后执行
`go -C tools/downkitdev run . deploy --os windows --arch amd64 --target /mnt/c/software/Coding/downkit`。Windows 安装注册执行 `bin\\downkitdev.exe install`，卸载注册执行 `bin\\downkitdev.exe uninstall`。Linux/macOS 则使用 `./bin/downkitdev install` 与 `uninstall`。

## 发行约束

`devtool package` 输出 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 发行包、SHA256 校验文件和组件清单。正式 Windows 包必须包含带许可证信息的 FFmpeg、Whisper、Llama 组件；缺失则拒绝打包。仅供开发的 `--allow-missing-tools` 必须显式指定。

Go 负责工作流编排，Android 仍使用官方 Gradle/NDK/gomobile，FFmpeg 仍使用上游 configure/make。仓库保留 Gradle 官方的 `gradlew` 和 `gradlew.bat`，避免造轮子。

TOML 配置、模型、日志及任务仍保留在安装目录 `Data/`。部署和安装不会删除用户数据。Android 与 FFmpeg 原生编译需要相应 SDK、NDK 或 MSYS2，只有在实际运行通过后才能认定对应平台的构建完成。
