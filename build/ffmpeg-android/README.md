# Android 精简 FFmpeg

这里构建的不是 `ffmpeg` 命令行，而是供 Android App 通过 JNI 调用的精简 libav `.so`。
默认只构建 `arm64-v8a`，只保留 DownKit 本地 HLS 清单无损封装 MP4 所需组件。

## 依赖

- Linux、macOS 或 WSL
- Android NDK
- FFmpeg 源码
- make

## 构建

```bash
export ANDROID_NDK_HOME=/path/to/android-ndk
./build.sh /path/to/ffmpeg
```

同时构建常见 Android ABI：

```bash
ANDROID_ABIS="arm64-v8a armeabi-v7a x86_64" ./build.sh /path/to/ffmpeg
```

产物位于 `out/jniLibs/<abi>/`。正式集成时复制到 Android 模块的
`src/main/jniLibs/<abi>/`，并由一个很薄的 JNI wrapper 调用 libavformat。

当前配置故意关闭网络和转码：网络下载、AES 解密由 Go 引擎负责，FFmpeg 只读取本地清单和分片并执行 stream copy。这样可以缩小体积，也不需要 x264/x265 等 GPL 编码器。

分发 App 时必须同时提供 FFmpeg/libav 的许可证、版本和对应源码获取方式；如果以后启用 GPL 组件，需要重新审查整个应用的许可证义务。
