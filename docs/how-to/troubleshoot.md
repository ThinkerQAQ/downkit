# Troubleshoot a first installation

This guide follows the order that errors appear during [first run](../tutorial/first-run-on-new-machine.md). Do not change the download engine, models or proxy settings before confirming that the browser can talk to the Bridge.

## Native Messaging host not found

The browser extension connects through the registered native host \`com.downkit.bridge\`, **not** by finding an arbitrary \`downkit\` process.

1. Open the **extracted package root** and run \`.\bin\downkitdev.exe install\` in Windows PowerShell or \`./bin/downkitdev install\` on Linux/macOS.
2. Confirm that the installer prints the executable path and the host manifest path under \`Data/native-host/\`.
3. In \`chrome://extensions\` or \`edge://extensions\`, inspect DownKit's extension ID. If it differs from \`hfjpenjlamneepmigemmmiibebpokmik\`, re-run \`downkitdev install --extension-id ACTUAL_EXTENSION_ID\`.
4. Click **Reload** on the unpacked extension, then open it again. If the package was moved, re-run install from its **new** root.

![A genuine browser state when the Native Messaging host cannot be found](../images/bridge-not-found-en.png)

*Actual DownKit v1.2.0 isolated Chromium screenshot showing the **failure** state; it is intentionally not labelled as a successful setup.*

## Bridge indicator remains Checking environment

Start by clicking the extension icon rather than opening the Web UI URL directly. The extension starts the Bridge on demand. Open **Settings → Connection & runtime** and refresh the health display.

Once the Bridge is started, you can check the local loopback endpoint at \`http://127.0.0.1:17891/\` from the **same computer**. A network response alone does not establish that the extension has Native Messaging permission or an authenticated relay. Do not expose this listener on a public address.

If there is a stale or incompatible process, stop it before reinstalling; do not delete \`Data/\`, which contains existing tasks and settings. Diagnostic logs are stored in \`Data/logs/\`.

## Detect shows No media found

- Return to the original webpage tab and **play** the permitted video; some websites do not request media until playback or seeking.
- Make sure you are looking at the same tab in which the media is playing.
- Look for full MP4, \`.m3u8\` or \`.mpd\` requests instead of individual segments.
- Pages with DRM, inaccessible cross-origin requests or proprietary delivery are not guaranteed to work.

An empty result is not the same as a failed download; no task has been submitted yet. Follow [First download](../tutorial/first-download.md) to select a resource.

## Task appears but fails

Open **Downloads** and inspect the failed stage and error. Check whether FFmpeg is available, whether the media URL/session has expired, whether a proxy is required, and whether the output directory is writable. The default **No subtitles** path does not need any local model; ASR requires whisper.cpp and a model, and translation requires llama.cpp and a model.

Do not press Download repeatedly for a timed-out task. Check whether a job already exists, then use the supported Retry/Resume action. Review logs in \`Data/logs/\` without posting credentials or signed media URLs.

## Workspace opens but cannot perform actions

The Web UI at \`http://127.0.0.1:17891/console/\` uses the browser extension as a relay for privileged requests. Open it with **Open workspace** in the extension. The page itself does not hold the Bridge token. Check extension permissions and connection state before treating a static page as a functioning downloader.