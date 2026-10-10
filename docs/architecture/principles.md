# Architecture principles

1. **Reuse mature components** (SDKs, protocols, libraries, tools) before implementing new ones.
2. **Single download engine:** desktop task state and settings belong in Go Bridge; UI surfaces share feature and transport logic.
3. **Explicit trust boundaries:** extension owns browser credentials and tokens; Web UI does not.
4. **Replaceable components:** FFmpeg, yt-dlp, whisper.cpp and llama.cpp are individually managed.
5. **Configuration-first design:** put extensions behind contracts and keep the core minimal.
6. **One Agent Contract:** DEV2 is the development entry point, with code and Markdown reviewed by appropriate intelligence services.
