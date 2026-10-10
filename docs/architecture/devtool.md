# DEVTool and Go maintenance

DownKit follows the same project-management pattern as BlogCTL: a root `.devtool.toml`, a self-contained `devcontrol/` Project Extension, one Go maintenance implementation, bilingual docs and a separate runtime UI. Development no longer depends on PowerShell, CMD or handwritten shell workflows.

## Directory responsibilities

| Directory | Owner |
| --- | --- |
| `bridge/` | Native Go downloader, authenticated loopback API, Native Messaging host and Android Go binding |
| `extension/` | Browser Manifest V3 UI, Popup, Side Panel and local Web UI relay |
| `mobile/android/` | Android/Kotlin UI, standard Gradle Wrapper and mobile deployment |
| `tools/downkitdev/` | Cross-platform Go command-line maintainer: build, verify, package, install, dependencies, FFmpeg and Android |
| `devcontrol/` | Thin DEVTool Project Extension that delegates to the Go maintainer |
| `docs/`, `docs/zh-CN/` | Documentation and aligned translations |
| `.devtool/out/`, `.devtool/artifacts/` | Local build products and release outputs (ignored by Git) |
| `bin/`, `extension/`, `tools/`, `Data/` in installed bundles | Installed executable and static assets, plus persistent local data |

`bridge/` is intentionally retained as its existing Go module. Moving it into another folder solely to mirror BlogCTL would disrupt existing Go bindings and runtime paths without architectural benefit.

## First commands

```sh
devtool config validate
devtool project inspect --json
devtool code doctor
devtool verify
devtool code verify
devtool build
devtool package
```

DEVTool commands are defined by `devcontrol/provider.go` and run the **same implementation** under `tools/downkitdev`. Environment and runtime providers remain configurable in `.devtool.toml`.

## Go maintainer commands

From a source checkout:

```sh
go -C tools/downkitdev run . verify
go -C tools/downkitdev run . build --os windows --arch amd64
go -C tools/downkitdev run . package
go -C tools/downkitdev run . components --root /path/to/installed/downkit
go -C tools/downkitdev run . tool-install whisper
go -C tools/downkitdev run . tool-install llama
go -C tools/downkitdev run . android-build
go -C tools/downkitdev run . ffmpeg-native --source /path/to/FFmpeg
go -C tools/downkitdev run . ffmpeg-android --source /path/to/FFmpeg
go -C tools/downkitdev run . icons
```

For a Windows deployment from WSL, first stop the running Bridge, then use
`go -C tools/downkitdev run . deploy --os windows --arch amd64 --target /mnt/c/software/Coding/downkit`. On Windows, register the new executable using `bin/downkitdev.exe install`. Native Messaging and URL protocol registration are owned by the Go tool; `bin/downkitdev.exe uninstall` removes registrations but preserves user data.

Installed Linux/macOS packages use `./bin/downkitdev install` and `./bin/downkitdev uninstall`.

## Release, safety and platform boundaries

`devtool package` builds Windows amd64, Linux amd64/arm64 and macOS amd64/arm64 archives with checksums and a generated `components.json`. A complete Windows production bundle requires licensed FFmpeg, whisper.cpp and llama.cpp sidecars in `dist/tools/`; missing components fail the release. `--allow-missing-tools` is explicit **development-only** behavior.

Go handles orchestration and artifact checks. Gradle, gomobile, FFmpeg configure/make, Android NDK and MSYS2 remain their mature upstream build engines. The checked-in Gradle Wrapper (`mobile/android/gradlew` and `gradlew.bat`) is vendor tooling, not a second custom maintenance system.

Installed runtime configuration is TOML at `Data/config.toml`. Jobs and browser protocol manifests use their required JSON formats. Generated files and downloads are not deleted by install or deploy.

The Windows and Linux build/verify flow has been exercised. Android builds require an Android SDK/NDK and JDK. FFmpeg source builds require their upstream native toolchains and pinned source; availability must be checked before claiming those actions succeeded.
