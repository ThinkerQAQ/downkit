# 配置参考

DownKit 的**所有可编辑运行配置与浏览器偏好**统一保存在安装根目录的 `Data/config.toml`，其中 `Data` 与 `bin` 同级。Bridge 状态、下载任务、日志、模型、按需下载工具和桌面任务工作目录也位于 `Data`。

| TOML 配置节 | 字段 | 用途 |
| --- | --- | --- |
| `[download]` | `output_dir`、`concurrent`、`quality` | 下载目录、并发上限（1–64）、默认画质 |
| `[proxy]` | `enabled`、`url`、`host`、`port` | 外网请求代理 |
| `[tools]` | `ffmpeg`、`yt_dlp`、`whisper`、`whisper_model`、`llama`、`translation_model` | 组件路径及模型 |
| `[interface]` | `open_mode`、`language` | Popup/侧边栏入口、中文或英文 |
| `[sniffer]` | `hls`、`dash`、`mp4` | 资源检测开关 |
| `[logging]` | `enabled`、`max_size_mb` | 日志开关和大小上限（1–100 MB） |

Bridge 负责校验与原子写入，浏览器通过扩展后台访问配置。扩展设置统一由 TOML 管理；标签页中短期缓存的媒体资源仍由浏览器会话存储承载。Native Messaging 按浏览器标准需要 JSON 注册清单，生成于 `Data/native-host/`，它不是用户配置。

自动迁移代码已移除，运行时直接读取 Data/config.toml。任务记录继续使用 JSON 状态快照。已经指定的下载文件夹不会被擅自移动。界面语言与字幕目标语言互相独立。

直接修改 TOML 文件后需要重启 Bridge；通过界面保存的配置会立即生效。
