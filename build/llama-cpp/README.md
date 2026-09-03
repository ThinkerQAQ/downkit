# llama.cpp Windows server

DownKit keeps the Windows x64 CPU server as a separate sidecar under
`dist/tools/llama/`. The standard release package copies that directory
without embedding it into the Bridge executable.

Provision the pinned official server with:

```powershell
.\build\llama-cpp\install-windows.ps1
```

The script downloads the official llama.cpp Windows CPU archive, verifies the
SHA-256 published by GitHub, and copies `llama-server.exe` with its required
CPU runtime libraries. Translation model files are deliberately excluded:
TranslateGemma uses the Gemma license, which the user must review separately.
