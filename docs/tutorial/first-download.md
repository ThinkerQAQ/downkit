# Your first download

**Goal:** create a download from a media-playing browser tab, then check its actual output. Start with [installation and first-run screenshots](first-run-on-new-machine.md) if you have not connected the extension.

## Before opening Detect

Choose a video you have permission to save. Prefer a straightforward MP4 or non-DRM HLS video for the first attempt. Leave the original tab **open and playing**: many pages only request the video after playback begins.

Click the DownKit toolbar icon. The navigation rail contains **Detect**, **Downloads**, and **Settings**. If the top-right status is not healthy, follow [connection troubleshooting](../how-to/troubleshoot.md) first.

## Find the correct resource

Go to **Detect**. Under **Resources on this page / Detected resources**, look for a complete media resource:

| What appears | What to choose |
| --- | --- |
| \`video.mp4\` | Usually the direct file |
| \`master.m3u8\` or a variant \`.m3u8\` | An HLS manifest; quality and playlist options depend on the source |
| \`manifest.mpd\` | A DASH manifest; may require separate audio/video handling |
| \`.ts\`, \`.m4s\` or isolated audio segments | Usually **not** the full video; find the parent manifest |

If nothing appears, play or seek the video once and open Detect again. Ads, unrelated page requests, and multiple qualities may appear in the list; inspect the name/URL and choose the intended item.

![Detect before a video has been observed](../images/first-run-detect-en.png)

*An actual empty state from v1.2.0, not a staged list of downloadable URLs.*

## Submit one job

Start with **Subtitles → No subtitles**, avoiding any local speech/translation dependencies. Click **Download** beside the intended resource **once**. On a playlist, confirm whether you are choosing a single item or the entire playlist. Do not assume every source can be saved; DRM or expired authorization can still prevent downloading.

Other subtitle modes use original site captions when available; local transcription needs whisper.cpp **and a model**; translation needs llama.cpp **and a model**. These settings affect processing time and prerequisites, not the extension interface language. Configure those after the first ordinary download works.

## Inspect progress and the file

Open **Downloads**. A submitted job should appear there; the status moves through resolution/downloading/processing stages where applicable. Wait for **Completed**. Use the output-file action if offered, or open the download directory configured in **Settings → Download capabilities**.

A job marked **Failed**, **Paused**, or **Processing** is not a finished file. Read the stage and error first, then use **Retry/Resume** only where supported. Check \`Data/logs/\` for Bridge-side details; the configured folder is described in [Configuration](../reference/configuration.md).

## After it works

Use **Open workspace** to work in the larger local Web UI, or set popup versus side panel under **Settings → Appearance & interaction**. For problems, use [Troubleshooting](../how-to/troubleshoot.md); for an M3U8-specific example, see [HLS example](../examples/m3u8-download.md).