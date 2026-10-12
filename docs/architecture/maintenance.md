# Go maintenance and development tools

DownKit uses one native Go maintainer for builds, checks, release packaging, installation and platform-tool integration: `tools/downkitdev/`. The application does not require a separate development gateway or project SDK.

## Responsibility boundaries

| Directory | Responsibility |
| --- | --- |
| `bridge/` | Desktop downloader, authenticated loopback API, Native Messaging host and Android Go bindings |
| `extension/` | Manifest V3 browser integration, Popup, Side Panel and local Web UI relay |
| `mobile/android/` | Kotlin app and upstream Gradle Wrapper |
| `tools/downkitdev/` | Build, verify, package, deploy, component management, Android and FFmpeg orchestration |
| `docs/`, `docs/zh-CN/` | English docs and aligned Chinese translations |
| `.build/out/`, `.build/artifacts/`, `.build/cache/` | Ignored, reproducible build outputs and caches |
| Installed `bin/`, `extension/`, `tools/`, `Data/` | Program files and persistent user data (never reset `Data/` during deploy) |

`bridge/` remains a separate Go module because its import and gomobile paths are already part of the product's toolchain.

## Native commands

Run at the checkout root:

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

`verify` runs tests for both Go modules, JavaScript syntax checks and browser-extension test scripts. It does **not** establish real browser, Android or Windows installation acceptance.

Default build output: `.build/out/`. Release archives: `.build/artifacts/release/v<version>/`. Android artifacts: `.build/artifacts/android/`. Generated assets and caches are ignored by Git.

## Language servers for maintainers and agents

- **Go:** use `gopls serve` through an LSP client, with `bridge/` and `tools/downkitdev/` as separate Go modules. Use symbols/definitions/references and diagnostics for targeted changes.
- **Browser code:** use installed editor language-server support where available. Otherwise use `rg`, source inspection, `node --check`, and browser tests.
- **Markdown:** `marksman server` provides LSP navigation and link references. Review prose and tables directly; a heading outline is insufficient for full-document review.

The language servers must be launched by an LSP client; CLI shells and plain text searches are valid fallbacks. No custom tool gateway is required.

## Release and deployment

`package` builds Windows amd64, Linux amd64/arm64 and macOS amd64/arm64 archives, checksums and `components.json`. The complete Windows release requires appropriately licensed FFmpeg, whisper.cpp and llama.cpp sidecars under `dist/tools/`; missing components fail packaging. `--allow-missing-tools` is only for deliberately incomplete development bundles, not published releases.

For Windows deployment from WSL, stop the Bridge before running:

```sh
go -C tools/downkitdev run . deploy --os windows --arch amd64 --target /mnt/c/software/Coding/downkit
```

Register Native Messaging with `bin/downkitdev.exe install` on Windows or `./bin/downkitdev install` on Linux/macOS. Uninstall with the corresponding `uninstall` command. Install/deploy leave `Data/`, existing downloads and saved configuration intact.

Android uses the official Gradle Wrapper, gomobile, SDK/NDK and JDK. FFmpeg uses upstream configure/make, Android NDK and MSYS2 as needed; no custom Shell, CMD or PowerShell orchestration replaces these toolchains. Verify the required SDK/toolchain before claiming a platform build succeeded.
