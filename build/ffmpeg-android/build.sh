#!/usr/bin/env bash
set -euo pipefail

# Build the small libav subset needed to remux DownKit local playlists.
# Usage: ANDROID_NDK_HOME=/path/to/ndk ./build.sh /path/to/ffmpeg [output-dir]

FFMPEG_SRC="${1:-}"
OUTPUT_DIR="${2:-$(pwd)/out}"
API="${ANDROID_API:-24}"
ABIS="${ANDROID_ABIS:-arm64-v8a}"

if [[ -z "$FFMPEG_SRC" || ! -x "$FFMPEG_SRC/configure" ]]; then
  echo "usage: ANDROID_NDK_HOME=/path/to/ndk $0 /path/to/ffmpeg [output-dir]" >&2
  exit 2
fi
if [[ -z "${ANDROID_NDK_HOME:-}" || ! -d "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt" ]]; then
  echo "ANDROID_NDK_HOME is missing or invalid" >&2
  exit 2
fi

case "$(uname -s)" in
  Linux*)  HOST_TAG="linux-x86_64" ;;
  Darwin*) HOST_TAG="darwin-x86_64" ;;
  *) echo "run this script on Linux or macOS (WSL is supported)" >&2; exit 2 ;;
esac

TOOLCHAIN="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$HOST_TAG"
mkdir -p "$OUTPUT_DIR"

build_abi() {
  local abi="$1" arch target extra_cflags=""
  case "$abi" in
    arm64-v8a)
      arch="aarch64"; target="aarch64-linux-android" ;;
    armeabi-v7a)
      arch="arm"; target="armv7a-linux-androideabi"
      extra_cflags="-march=armv7-a -mfloat-abi=softfp -mfpu=neon" ;;
    x86_64)
      arch="x86_64"; target="x86_64-linux-android" ;;
    *) echo "unsupported ABI: $abi" >&2; exit 2 ;;
  esac

  local prefix="$OUTPUT_DIR/prefix/$abi"
  local build_dir="$OUTPUT_DIR/build/$abi"
  rm -rf "$prefix" "$build_dir"
  mkdir -p "$prefix" "$build_dir"

  pushd "$build_dir" >/dev/null
  "$FFMPEG_SRC/configure" \
    --prefix="$prefix" \
    --target-os=android \
    --arch="$arch" \
    --enable-cross-compile \
    --cc="$TOOLCHAIN/bin/${target}${API}-clang" \
    --cxx="$TOOLCHAIN/bin/${target}${API}-clang++" \
    --ar="$TOOLCHAIN/bin/llvm-ar" \
    --nm="$TOOLCHAIN/bin/llvm-nm" \
    --ranlib="$TOOLCHAIN/bin/llvm-ranlib" \
    --strip="$TOOLCHAIN/bin/llvm-strip" \
    --extra-cflags="-fPIC -Os $extra_cflags" \
    --enable-shared \
    --disable-static \
    --disable-programs \
    --disable-doc \
    --disable-debug \
    --disable-autodetect \
    --disable-network \
    --disable-avdevice \
    --disable-postproc \
    --disable-swscale \
    --disable-swresample \
    --disable-everything \
    --enable-demuxer=hls,mpegts,mov,aac \
    --enable-muxer=mp4 \
    --enable-protocol=file,crypto,data \
    --enable-parser=aac,h264,hevc \
    --enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb

  make -j"${JOBS:-4}"
  make install
  popd >/dev/null

  local jni_dir="$OUTPUT_DIR/jniLibs/$abi"
  mkdir -p "$jni_dir"
  cp "$prefix/lib/"*.so "$jni_dir/"
  "$TOOLCHAIN/bin/llvm-strip" "$jni_dir/"*.so
  echo "built $abi -> $jni_dir"
}

for abi in $ABIS; do
  build_abi "$abi"
done
