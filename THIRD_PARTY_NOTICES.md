# Third-party notices

The Apache-2.0 license in `LICENSE` covers DownKit's original source code and documentation. It does not replace the licenses of the components listed below.

## FFmpeg

Desktop release packages may include a separate FFmpeg Slim executable built from FFmpeg 8.1.2. The current build disables GPL and nonfree components and reports `LGPL-2.1-or-later`.

Each release that bundles FFmpeg must also include:

- the LGPL 2.1 license text;
- the exact corresponding FFmpeg source archive;
- the build configuration and any DownKit-specific changes;
- a source and license notice next to the download.

FFmpeg source and license information: <https://ffmpeg.org/legal.html>

## yt-dlp

yt-dlp is an optional external tool. DownKit does not bundle it in standard releases; the user can request an authenticated download from the official yt-dlp release page. The upstream source is published under the Unlicense, while some packaged executables include components under additional licenses, including GPL-3.0-or-later for PyInstaller-based executables.

Upstream project and licensing information: <https://github.com/yt-dlp/yt-dlp>

## golang.org/x/mobile

The Android bridge uses `golang.org/x/mobile`, licensed under the BSD 3-Clause License.

Upstream license: <https://github.com/golang/mobile/blob/master/LICENSE>

## Gradle Wrapper and JUnit

The Android project includes the Gradle Wrapper under Apache-2.0 and uses JUnit 4.13.2 for tests under Eclipse Public License 1.0. These tools do not change the license of DownKit's original code.
