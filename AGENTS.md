# DownKit — Agent Contract

DownKit is a browser-integrated media downloader. The browser extension owns page detection, cookies and authorization. The local Go Bridge owns desktop task state, downloading, persistence and media tool orchestration. Android is a separate native client.

## First minute

Run from the repository root:

```bash
devtool version
devtool config validate
devtool project inspect --json
devtool code doctor
```

Use DEV2-discovered project commands (`devtool verify`, `devtool build`, `devtool package`) instead of an independent build pipeline. For code understanding use the DEV2 `code_context` capability; for documentation use `document_context` with bounded review and follow its cursor. Review Markdown through Goldmark/Marksman, not `devtool code verify`.

## Architecture invariants

- Mature components first: inspect existing SDKs, protocols, open source and media tools before building new ones.
- Minimal Go core with modular, replaceable tool integrations. No duplicated feature state in Popup, Side Panel or Web UI.
- Browser page sessions stay in the extension. Never send Native Messaging tokens, cookies or credentials to Web UI assets, storage, logs or docs.
- Web UI may serve shared extension assets from loopback, but privileged actions must cross the extension relay.
- Keep tool configuration explicit. Distinguish read-only detection from downloading, file deletion and other side effects.
- Keep interface locale separate from ASR language, subtitle language and downloaded media.
- Respect platform capability differences; desktop Bridge and Android are separate runtimes.

## Source map

- Desktop API and downloads: `bridge/`
- Browser host and UI: `extension/background.js`, `extension/popup/`, `extension/console-relay.js`
- Android: `mobile/android/`
- DEV2 Project Extension: `devcontrol/`
- Entry documentation: [docs/index.md](docs/index.md), [中文文档](docs/zh-CN/index.md)
- API details: [docs/reference/bridge-api.md](docs/reference/bridge-api.md)

## Verification and delivery

Inspect first, make focused changes, run DEV2 verification and test relevant browser surfaces. Android verification needs the Android SDK and appropriate build environment.

For changes run `devtool verify` and `devtool code verify`. DEVTool delegates to `tools/downkitdev`, the single Go maintenance implementation; `devtool package` builds release archives. Do not invoke removed PowerShell/Shell launchers.

Reload the unpacked browser extension after JavaScript, CSS, manifest or locale changes; restarting Bridge does not reload extension assets. Do not commit or push unless explicitly requested.

## Local configuration and data

Installed binaries reside in `bin/`; local runtime files are stored in the sibling `Data/` directory. Edit `Data/config.toml` for the download, proxy, tools, interface, sniffer and logging settings. Job state (`jobs.json`), Bridge runtime state (`bridge.json`), logs, models, downloaded tool components and staging checkpoints live under `Data/`. Native Messaging requires a JSON registration manifest at `Data/native-host/com.downkit.bridge.json`; it is generated protocol metadata, not a user config file. Existing download destinations remain unchanged when migrating legacy configuration.

## Go-only maintenance workflow

- Use `devtool verify`, `devtool build`, `devtool package`, `devtool code verify` and `devtool project inspect --json` as authoritative engineering entry points.
- DEVTool Project Extension lives in `devcontrol/` and delegates operations to `tools/downkitdev/`. Maintain only one implementation of build, packaging, install, dependency resolution and asset generation in Go.
- Official Gradle Wrapper remains for Android. FFmpeg configure/make, MSYS2, Android SDK/NDK and gomobile remain upstream toolchains invoked from Go. No custom PowerShell, Shell or CMD orchestration is kept.
- Windows packages install with `bin/downkitdev.exe install` and uninstall with `bin/downkitdev.exe uninstall`. The Go deployer must never overwrite the installed `Data/`.
- See [DEVTool architecture](docs/architecture/devtool.md) before changing project tasks.
