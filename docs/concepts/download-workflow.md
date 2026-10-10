# Download workflow

**Detect → resolve → download → mux → optional subtitles → output.**

The extension detects HLS, DASH and MP4 and submits the resource with permitted page context. The Go Bridge resolves and downloads the resource, persists checkpoints and reports progress. FFmpeg handles muxing. whisper.cpp handles local transcription and llama.cpp handles optional translation.

Deleting the record and deleting local files require distinct user choices. Browser credentials must not be stored in persisted job records.
