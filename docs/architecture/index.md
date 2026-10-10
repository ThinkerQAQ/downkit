# Architecture

Read [principles](principles.md) before refactoring.

Entry points: `bridge/bridge.go` (control API), `bridge/web_console.go` (local UI assets), `extension/background.js` (privileged browser host), `extension/popup/` (shared UI) and `devcontrol/` (DEV2 extension).

- [DEVTool and Go maintenance](devtool.md)
