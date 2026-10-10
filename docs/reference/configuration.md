# Configuration

DownKit keeps all editable desktop and browser preferences in **Data/config.toml**, next to the installed **bin/** directory. This file is TOML, not JSON. Bridge state, jobs, logs, models, downloaded tools and desktop work checkpoints also live under **Data/**.

| TOML section | Keys | Purpose |
| --- | --- | --- |
| `[download]` | `output_dir`, `concurrent`, `quality` | Media destination, max workers (1–64), and default quality |
| `[proxy]` | `enabled`, `url`, `host`, `port` | External network proxy |
| `[tools]` | `ffmpeg`, `yt_dlp`, `whisper`, `whisper_model`, `llama`, `translation_model` | Optional tool overrides and model files |
| `[interface]` | `open_mode`, `language` | Popup or side panel, `zh-CN` or `en` |
| `[sniffer]` | `hls`, `dash`, `mp4` | Browser media detection categories |
| `[logging]` | `enabled`, `max_size_mb` | Local runtime logs (1–100 MB per active file) |

The Bridge validates writes and saves changes atomically. The browser extension talks to its authenticated API; detected tab resources remain short-lived browser session cache, not user configuration. Native Messaging itself requires a generated JSON registration manifest under `Data/native-host/`.


Edits made directly to TOML require restarting the Bridge; changes saved through the UI apply immediately.
