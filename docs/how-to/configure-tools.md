# Configure tools

Open **Settings**. Inspect health and edit the relevant card.

- Go downloader: output directory, concurrency (1–64), quality, proxy.
- FFmpeg: muxing, bundled on Windows and installed separately on Linux/macOS.
- yt-dlp: optional webpage resolver.
- whisper.cpp: optional speech recognition; download model weights separately.
- llama.cpp: optional translation; review and accept model licensing.

Desktop configuration lives beside the installation bin/ directory in `Data/config.toml`. Sniffer and interface preferences are stored in TOML. Transient media detections stay in browser session storage.
