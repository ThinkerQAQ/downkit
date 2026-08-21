#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: build-native.sh FFMPEG_SOURCE BUILD_DIR OUTPUT_DIR" >&2
  exit 2
fi

source_dir="$(cd "$1" && pwd)"
build_dir="$2"
output_dir="$3"

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
  --enable-muxer=mp4,mov,matroska,webm,adts \
  --enable-parser=h264,hevc,av1,vp8,vp9,aac,ac3,opus,vorbis \
  --enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb,vp9_superframe \
  --enable-decoder=h264,hevc,aac,mp3,ac3,eac3,flac,opus,vorbis,vp8,vp9,av1,wrapped_avframe \
  --enable-filter=null,anull \
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
