# 架构

先阅读[架构原则](principles.md)。

代码入口：`bridge/bridge.go`（控制接口）、`bridge/web_console.go`（Web 静态资源）、`extension/background.js`（可信浏览器后台）、`extension/popup/`（共享 UI）、`devcontrol/`（DEV2）。

- [DEVTool 与 Go 维护工具](devtool.md)
