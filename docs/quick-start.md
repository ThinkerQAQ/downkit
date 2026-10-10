# Quick Start

## Requirements

- Chrome, Chromium, or Edge with permission to load an unpacked extension
- A DownKit desktop release for Windows, Linux, or macOS
- FFmpeg on Linux/macOS, available through PATH; Windows release bundles a slim build

## Install

1. Download and extract the matching archive from [GitHub Releases](https://github.com/ThinkerQAQ/downkit/releases). Keep the Bridge binary, `extension/` and `tools/` together.
2. On Windows, run `bin/downkitdev.exe install`; on Linux/macOS, run `./bin/downkitdev install`.
3. Open `chrome://extensions` or `edge://extensions`, enable Developer mode and load the unpacked `extension/` folder.
4. Open a video page and start playback. Click DownKit, inspect **Detect**, and choose **Download**.

The extension starts the local Bridge through Native Messaging when needed. It does not require a permanent terminal process.

[Next: your first download](tutorial/first-download.md)
