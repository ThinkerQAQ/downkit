# 快速开始

**第一次使用请先阅读带截图的[新电脑首次使用教程](tutorial/first-run-on-new-machine.md)。** 它会说明安装、浏览器授权、连接状态和无资源时的排查方式。

## 5 步开始下载

1. 从 [Releases](https://github.com/ThinkerQAQ/downkit/releases) 下载适合 Windows / Linux / macOS 的发行包，**完整解压到长期使用的目录**。
2. 在该目录通过 PowerShell 执行 \`.\bin\downkitdev.exe install\`，或在 Linux/macOS 执行 \`./bin/downkitdev install\`。Linux/macOS 需要自行安装 FFmpeg。
3. 打开 \`chrome://extensions\` 或 \`edge://extensions\`，启用「开发者模式」，选择「加载已解压的扩展程序」，指向 \`extension/\`。
4. 固定并打开 DownKit，确认环境状态正常；不正常先看[连接故障排查](how-to/troubleshoot.md)。
5. 播放有权保存的视频，选择「资源嗅探 → 资源 → 下载」，最后在「下载任务」确认状态为「已完成」。

![资源嗅探初始状态](../images/first-run-detect-en.png)

*真实 v1.2.0 初始界面，隔离测试下暂无资源，不代表下载已经成功。*

下一步：[首次下载](tutorial/first-download.md)，含选资源、字幕与任务状态的解释。

更多入口：[新电脑首次使用](tutorial/first-run-on-new-machine.md) · [Web UI](how-to/use-web-ui.md) · [工具配置](how-to/configure-tools.md) · [故障排查](how-to/troubleshoot.md)。

使用发行包**不需要 DEVTool**；从源码开发请先看 [AGENTS.md](../../AGENTS.md) 和 [DEVTool](architecture/devtool.md)。