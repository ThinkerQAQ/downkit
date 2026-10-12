# Architecture

Read [principles](principles.md) before refactoring.

Entry points: `bridge/bridge.go` (control API), `bridge/web_console.go` (local UI assets), `extension/background.js` (privileged browser host), `extension/popup/` (shared UI) . The native engineering tool is `tools/downkitdev/`.

- [Go maintenance and native tools](maintenance.md)
