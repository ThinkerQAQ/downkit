#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: build-native.sh FFMPEG_SOURCE BUILD_DIR OUTPUT_DIR FFMPEG_VERSION" >&2
  exit 2
fi

source_dir="$(cd "$1" && pwd)"
build_dir="$2"
output_dir="$3"
ffmpeg_version="$4"

# Git's repository discovery otherwise walks up into the DownKit checkout and
# makes FFmpeg report the DownKit tag (for example v1.0.8) as its own version.
# Release archives do not contain this file, but FFmpeg's version generator
# intentionally prefers it over a discovered Git revision.
printf '%s\n' "$ffmpeg_version" > "$source_dir/VERSION"

mkdir -p "$build_dir" "$output_dir"
cd "$build_dir"

if [[ -f Makefile ]]; then
  make distclean
fi

"$source_dir/configure" \
  --disable-everything \
  --disable-autodetect \
  --disable-network \
  --disable-doc \
  --disable-debug \
  --disable-programs \
  --enable-ffmpeg \
  --enable-small \
  --enable-static \
  --disable-shared \
  --enable-protocol=file,pipe,crypto,data \
  --enable-demuxer=hls,mpegts,mov,matroska,webm,aac,mp3,ac3,eac3,ogg,flac,wav \
  --enable-muxer=mp4,mov,matroska,webm,adts,wav \
  --enable-encoder=pcm_s16le \
  --enable-parser=h264,hevc,av1,vp8,vp9,aac,ac3,opus,vorbis \
  --enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb,vp9_superframe \
  --enable-decoder=h264,hevc,aac,mp3,ac3,eac3,flac,opus,vorbis,vp8,vp9,av1,wrapped_avframe \
  --enable-filter=null,anull,aresample,aformat \
  --extra-cflags='-Os -ffunction-sections -fdata-sections' \
  --extra-ldflags='-static -static-libgcc -Wl,--gc-sections'

make -j"$(nproc 2>/dev/null || echo 2)"

binary="$build_dir/ffmpeg"
suffix=""
if [[ -f "$build_dir/ffmpeg.exe" ]]; then
  binary="$build_dir/ffmpeg.exe"
  suffix=".exe"
fi

strip "$binary" 2>/dev/null || true
cp -f "$binary" "$output_dir/ffmpeg-slim$suffix"
