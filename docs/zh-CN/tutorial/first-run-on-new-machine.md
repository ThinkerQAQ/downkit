# 在新电脑上首次使用 DownKit

**目标：** 从下载发行包开始，安装本地 Bridge、加载浏览器扩展、找到视频资源，并确认下载文件的位置。使用发行包不需要安装 Go、Node.js，也不需要账号或语音识别模型。

本文对应 **DownKit v1.2.0**。图片来自隔离 Chromium 环境中实际运行的 v1.2.0 英文扩展。该隔离环境 **未成功连接 Native Messaging**，因此图片用于说明导航与初始界面，并非已连接、已嗅探或下载成功的证据。

## 第一步：下载并解压

打开 [GitHub Releases](https://github.com/ThinkerQAQ/downkit/releases)，选择与电脑操作系统及处理器匹配的压缩包，解压到准备长期使用的目录。

- \`bin/\`：Bridge 和 Go 安装工具
- \`extension/\`：供 Chrome / Edge 加载的扩展
- \`tools/\`：发行包附带的媒体工具
- \`Data/\`：运行时生成的设置、任务、日志等持久数据

**不要**直接加载 ZIP 文件，不要单独移动 \`bin/\`，更新时保留 \`Data/\`。Windows 发行包携带 FFmpeg；Linux/macOS 需要自行将 FFmpeg 安装到 \`PATH\`。语音识别和翻译模型是可选组件。

## 第二步：注册本地 Bridge

在**解压后的根目录**打开终端：

Windows PowerShell：

~~~powershell
.\bin\downkitdev.exe install
~~~

Linux/macOS：

~~~sh
./bin/downkitdev install
~~~

正常情况下会输出 \`Native Messaging installed\` 和 \`Host manifest\` 的路径。安装过程用于注册 \`com.downkit.bridge\`，无需常驻打开一个终端；扩展在需要时启动 Bridge。

如果实际加载的扩展 ID 与内置 ID 不同，可通过 \`--extension-id YOUR_32_CHARACTER_ID\` 重新注册。安装报错时先阅读[连接排查](../how-to/troubleshoot.md)，不要继续尝试下载。

## 第三步：在浏览器加载扩展

1. 打开 \`edge://extensions\` 或 \`chrome://extensions\`。
2. 打开「开发者模式」。
3. 点击「加载已解压的扩展程序」，选择**包含 \`manifest.json\` 的 \`extension/\` 文件夹**。
4. 将 DownKit 固定到工具栏，点击图标。

扩展主要有三个入口：**Detect（资源嗅探）、Downloads（下载任务）、Settings（设置）**。右上角环境状态应最终变为正常。如果持续显示 **Checking environment** 或 **Environment issue**，先检查 Bridge，不要将它视为安装成功。

![DownKit v1.2.0 首次打开资源嗅探页面](../../images/first-run-detect-en.png)

*真实 v1.2.0 英文界面；此处「No media found」是空状态，截图本身不代表 Bridge 已连接。*

## 第四步：检查语言、连接与工具

打开 **Settings → Appearance & interaction → Interface language**，可以选「中文」或「English」。这里修改的是软件界面语言，**不等于视频语音语言或字幕翻译目标语言**；同一页面也可以选择弹窗或侧边栏打开方式。

![v1.2.0 设置页面导航](../../images/first-run-settings-en.png)

*真实英文设置导航，环境连接状态需在实际安装后的浏览器中另行确认。*

在 **Settings → Connection & runtime** 检查本地连接；下载工具在 **Download capabilities** 中。第一次下载推荐将字幕设置为 **No subtitles（不下载字幕）**，这样不依赖 whisper.cpp / llama.cpp 模型。

## 第五步：嗅探并下载一个有保存权限的视频

1. 打开自己有权保存的视频网页，**先播放视频**。
2. 不要关闭原视频标签页，打开 DownKit，进入「资源嗅探」。
3. 在检测结果里找到完整的 MP4、HLS（\`.m3u8\`）或 DASH（\`.mpd\`）资源；同一网页可能列出多个清晰度、音轨或分片，优先选择完整媒体或播放清单，而不是单个分片。
4. 字幕选择「不下载字幕」，点击对应资源的「下载」。

**预期结果：**「下载任务」出现一个新任务。如果没有发现资源，返回原网页播放或拖动一次进度条，再次打开扩展。有 DRM 的内容或特殊站点不保证支持；**能检测到地址不代表一定能下载**。具体操作见[首次下载教程](first-download.md)。

## 第六步：确认文件已经完成

进入「下载任务」，找到刚创建的任务。状态必须显示**已完成**才算下载成功；然后使用打开输出文件的操作，或到「设置 → 下载能力」里配置的目录查找。若任务失败，应先读取具体错误，而不是反复重复下载。

点击扩展顶部的 **Open workspace（打开工作台）**，可在大页面使用 \`http://127.0.0.1:17891/console/\`。这个地址只在本机提供服务；Web UI 的授权操作仍依赖浏览器扩展转发，**单独打开网址并不等于扩展已经连接**。

## 常见问题

- **Native Messaging host not found / Environment issue：** 重新执行安装命令，确认扩展 ID，重载扩展。
- **一直显示 Checking environment：** 在设置中刷新连接状态，先排查 Bridge。
- **没有嗅探到资源：** 回到原视频标签页继续播放，再打开扩展。
- **提示缺少 FFmpeg：** 在下载能力里检查工具，Linux/macOS 需要自行安装。
- **任务失败：** 查看错误与 \`Data/logs/\`，不要将任务创建等同于下载成功。

更多见[故障排查](../how-to/troubleshoot.md)和[配置说明](../reference/configuration.md)。

## 复现记录

2026-10-11 曾使用当时的开发工具完成源码检查和 Linux Bridge 构建（现已切换为原生 Go 维护程序）；在临时 HOME 与 Data 目录下验证了安装命令，并用隔离 Chromium 加载 v1.2.0 英文扩展。由于这个隔离浏览器的 Native Messaging 未接通，图片记录的是**真实初始/未连接界面**，没有伪造媒体资源或任务，也**没有宣称完整下载链路通过**。