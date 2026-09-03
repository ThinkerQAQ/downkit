# whisper.cpp Windows client

DownKit keeps the Windows x64 CPU client as a separate sidecar under
`dist/tools/whisper/`. The standard release package copies that directory
without embedding it into the Bridge executable.

Provision the pinned official client with:

```powershell
.\build\whisper-cpp\install-windows.ps1
```

The script downloads `whisper-bin-x64.zip` from the official whisper.cpp
release, verifies the SHA-256 published by GitHub for that release asset, and
copies only the CLI and its CPU runtime libraries. Model files are deliberately
excluded because they are large and users may choose different multilingual
models.
