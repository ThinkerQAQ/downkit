# Quick Start

**If you are installing DownKit for the first time, start with [First run on a new machine](tutorial/first-run-on-new-machine.md).** It explains the release layout, browser permissions, connection status, language settings, and what to do when nothing appears.

## Desktop in five minutes

1. Download the Windows, Linux, or macOS archive from [Releases](https://github.com/ThinkerQAQ/downkit/releases) and extract the **whole** archive to a permanent folder.
2. From that folder, run \`.\bin\downkitdev.exe install\` in Windows PowerShell or \`./bin/downkitdev install\` on Linux/macOS. Linux/macOS require FFmpeg installed separately.
3. Open \`chrome://extensions\` or \`edge://extensions\`, enable **Developer mode**, select **Load unpacked**, and choose the extracted \`extension/\` folder.
4. Pin and open DownKit. Confirm its environment indicator is healthy; use [connection troubleshooting](how-to/troubleshoot.md) if it is not.
5. Play a video you are allowed to save, choose **Detect → resource → Download**, and wait for **Downloads → Completed**.

![Detect is where available media will appear after playback](images/first-run-detect-en.png)

*Actual v1.2.0 initial state. This isolated screenshot shows no detected media; it is not a successful-download example.*

For a careful explanation of resource selection and job status, follow [First download](tutorial/first-download.md).

## Which page should I read next?

- [First run](tutorial/first-run-on-new-machine.md): install a release with screenshots
- [First download](tutorial/first-download.md): follow a real media download
- [Web UI](how-to/use-web-ui.md): open the full-page workspace
- [Tools](how-to/configure-tools.md): choose FFmpeg, yt-dlp, subtitles and models
- [Troubleshooting](how-to/troubleshoot.md): resolve connection, empty resources, and failed jobs

A release requires no development tools. Developers should start with [AGENTS.md](../AGENTS.md) and [Go maintenance](architecture/maintenance.md).