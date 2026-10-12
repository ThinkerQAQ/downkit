# DownKit — Agent Contract

DownKit is a browser-integrated media downloader. The browser extension owns page detection, cookies and authorization. The local Go Bridge owns desktop jobs, downloads, persistence and media tools. Android is an independent native client.

## Start here

Run from the repository root:

```sh
go -C tools/downkitdev run . verify
go -C tools/downkitdev run . build
```

The Go maintenance tool in `tools/downkitdev/` is the **only** implementation of project build, verification, packaging, deployment and tool installation. Inspect `docs/architecture/maintenance.md` for all commands and platform requirements; do not add a second build wrapper.

## Code intelligence and direct tools

Prefer native LSP capabilities when your editor or Agent exposes an LSP client:

| Language | Server | Scope and useful operations |
| --- | --- | --- |
| Go | `gopls serve` | `bridge/`, `tools/downkitdev/` (separate Go modules); symbols, definitions, references, implementations, rename and diagnostics |
| JavaScript/HTML/CSS | Editor's installed language servers, if available | `extension/`; navigation, references and diagnostics (check which languages and features the client supports) |
| Markdown | `marksman server` | `README*.md`, `AGENTS.md`, `docs/`; headings, links, definitions and references |

These servers use the Language Server Protocol over standard I/O and require an **LSP client**. Do not treat the binaries as one-shot analysis commands, or claim a provider is configured when it is not. For Go, `gopls symbols`, `gopls definition`, `gopls references` and `gopls check` can also be used directly after checking their argument formats. For browsers without an installed JS language server, use source reads, `rg`, `node --check` and the existing JS tests rather than creating a new adapter. Review long Markdown files section by section; a symbol outline alone is not a complete review.

Use standard `git status/diff/log`, `rg`, and file reads/edits for repository work. Avoid duplicate code indexes or project-specific gateways. Tool failures must be reported, not silently substituted with invented success.

## Architecture invariants

- Reuse mature tools and SDKs before implementing infrastructure.
- Keep the Go core minimal, modular and replaceable. Popup, Side Panel and Web UI share task and settings state.
- Browser page sessions remain in the extension. Never copy Native Messaging secrets, cookies or authorization material into Web UI assets, persistent storage, logs or documentation.
- The Web UI may serve shared extension assets on loopback; privileged operations must cross the extension relay.
- Keep tool configuration explicit. Separate read-only detection from downloading, file deletion and other side effects.
- Interface locale, ASR language, subtitle language and downloaded media settings are separate.
- Desktop Bridge and Android are different runtimes with different upstream capabilities.

## Source map

- Desktop Bridge/API and downloader: `bridge/`
- Browser host, UI and tests: `extension/background.js`, `extension/popup/`, `extension/test/`
- Android: `mobile/android/`
- Native Go maintenance and release logic: `tools/downkitdev/`
- Entry documentation: [English](docs/index.md) · [中文](docs/zh-CN/index.md)
- API reference: [Bridge API](docs/reference/bridge-api.md)

## Build, verification and deployment

```sh
go -C tools/downkitdev run . verify
go -C tools/downkitdev run . build --os windows --arch amd64
go -C tools/downkitdev run . package
# Explicitly incomplete development release only:
go -C tools/downkitdev run . package --allow-missing-tools
```

`verify` runs Go tests, browser JavaScript syntax checks and the extension's JS tests. It is not a substitute for browser UI or Android-device acceptance. Formal release packaging requires verified third-party sidecars and source/licence assets; never silently switch to `--allow-missing-tools` for publishing.

Android uses the official Gradle Wrapper, Go mobile toolchain and Android SDK/NDK. FFmpeg uses upstream configure/make and native toolchains. Deploying to Windows is an explicit side effect: stop the running Bridge before `go -C tools/downkitdev run . deploy --os windows --arch amd64 --target /mnt/c/software/Coding/downkit`; never overwrite `Data/`. Reload the unpacked browser extension after HTML, JS, CSS, manifest or locale changes; a Bridge restart alone does not reload those assets.

Keep changes small, inspect the diff, commit/push coherent work when requested, and verify before delivery. Do not merge without explicit approval.

## Local configuration and data

Installed binaries are under `bin/`. `Data/config.toml` is the durable product configuration; `Data/jobs.json`, `Data/bridge.json`, `Data/logs/`, `Data/models/`, `Data/tools/` and `Data/work/` hold persistent or generated runtime data. `Data/native-host/com.downkit.bridge.json` is browser protocol metadata, not user configuration. Build outputs and caches live under ignored `.build/`; these must never be confused with installed `Data/`.

The Go installer registers Native Messaging and the URL protocol. Installing, deploying, or uninstalling program registrations must not erase existing downloads or user data.
