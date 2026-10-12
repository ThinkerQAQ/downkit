package maint

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func androidEnvironment() (string, string, string, error) {
	java := os.Getenv("JAVA_HOME")
	if java == "" {
		javac, err := exec.LookPath(tool("javac"))
		if err != nil {
			return "", "", "", errors.New("Android build requires JDK 17+")
		}
		java = filepath.Dir(filepath.Dir(javac))
	}
	sdk := os.Getenv("ANDROID_SDK_ROOT")
	if sdk == "" {
		sdk = os.Getenv("ANDROID_HOME")
	}
	if sdk == "" && runtime.GOOS == "windows" {
		sdk = filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk")
	}
	if _, err := os.Stat(sdk); err != nil {
		return "", "", "", errors.New("Android SDK not found (set ANDROID_SDK_ROOT)")
	}
	ndk := os.Getenv("ANDROID_NDK_HOME")
	if ndk == "" {
		roots, err := os.ReadDir(filepath.Join(sdk, "ndk"))
		if err != nil {
			return "", "", "", fmt.Errorf("Android NDK not found: %w", err)
		}
		for index := len(roots) - 1; index >= 0; index-- {
			candidate := filepath.Join(sdk, "ndk", roots[index].Name())
			if _, err := os.Stat(filepath.Join(candidate, "source.properties")); err == nil {
				ndk = candidate
				break
			}
		}
	}
	if ndk == "" {
		return "", "", "", errors.New("Android NDK required for gomobile")
	}
	return java, sdk, ndk, nil
}
func AndroidBind(root string) error {
	java, sdk, ndk, err := androidEnvironment()
	if err != nil {
		return err
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		cmd := exec.Command("go", "env", "GOPATH")
		out, err := cmd.Output()
		if err != nil {
			return err
		}
		gopath = strings.TrimSpace(string(out))
	}
	gomobile := filepath.Join(gopath, "bin", tool("gomobile"))
	gobind := filepath.Join(gopath, "bin", tool("gobind"))
	for _, bin := range []string{gomobile, gobind} {
		if _, err := os.Stat(bin); err != nil {
			return fmt.Errorf("missing %s; install pinned x/mobile tools", bin)
		}
	}
	source, err := os.ReadFile(filepath.Join(root, "bridge", "go.mod"))
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(source), "\n") {
		if strings.Contains(line, "golang.org/x/mobile") {
			words := strings.Fields(line)
			if len(words) >= 2 {
				expected = words[1]
			}
			break
		}
	}
	if expected == "" {
		return errors.New("bridge/go.mod missing x/mobile version")
	}
	for _, bin := range []string{gomobile, gobind} {
		output, err := exec.Command("go", "version", "-m", bin).Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(output), "golang.org/x/mobile\t"+expected) {
			return fmt.Errorf("%s version does not match golang.org/x/mobile %s", bin, expected)
		}
	}
	if _, err := os.Stat(filepath.Join(sdk, "platforms", "android-35", "android.jar")); err != nil {
		return errors.New("Android platform 35 SDK missing")
	}
	output := filepath.Join(root, "mobile", "android", "app", "libs", "downkit.aar")
	if err := ensureDir(filepath.Dir(output)); err != nil {
		return err
	}
	env := []string{"JAVA_HOME=" + java, "ANDROID_HOME=" + sdk, "ANDROID_SDK_ROOT=" + sdk, "ANDROID_NDK_HOME=" + ndk}
	return run(filepath.Join(root, "bridge"), env, gomobile, "bind", "-target", "android/arm64", "-androidapi", "26", "-o", output, ".")
}
func AndroidBuild(root string) error {
	if err := AndroidBind(root); err != nil {
		return err
	}
	gradle := "./gradlew"
	if runtime.GOOS == "windows" {
		gradle = "gradlew.bat"
	}
	if err := run(filepath.Join(root, "mobile", "android"), nil, gradle, "--no-daemon", "assembleDebug"); err != nil {
		return err
	}
	src := filepath.Join(root, "mobile", "android", "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	dst := filepath.Join(root, ".build", "artifacts", "android", "downkit-android-arm64-debug.apk")
	if err := copyFile(src, dst); err != nil {
		return err
	}
	hash, err := digest(dst)
	if err != nil {
		return err
	}
	fmt.Println("Android debug:", dst, hash)
	return nil
}
func FFmpegNative(root, source, output, version string) error {
	if runtime.GOOS == "windows" {
		return FFmpegWindows(root, source, output, version)
	}
	if source == "" {
		return errors.New("ffmpeg-native requires --source with an FFmpeg source tree")
	}
	if version == "" {
		version = "n8.1.2"
	}
	if output == "" {
		output = filepath.Join(root, "dist", "tools")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(source, "configure")); err != nil {
		return errors.New("FFmpeg configure missing")
	}
	work := filepath.Join(root, ".build", "cache", "ffmpeg-slim", "build")
	if err := ensureDir(work); err != nil {
		return err
	}
	if err := ensureDir(output); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	configure := []string{
		"--disable-everything", "--disable-autodetect", "--disable-network", "--disable-doc", "--disable-debug",
		"--disable-programs", "--enable-ffmpeg", "--enable-small", "--enable-static", "--disable-shared",
		"--enable-protocol=file,pipe,crypto,data", "--enable-demuxer=hls,mpegts,mov,matroska,webm,aac,mp3,ac3,eac3,ogg,flac,wav",
		"--enable-muxer=mp4,mov,matroska,webm,adts,wav", "--enable-encoder=pcm_s16le",
		"--enable-parser=h264,hevc,av1,vp8,vp9,aac,ac3,opus,vorbis",
		"--enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb,vp9_superframe",
		"--enable-decoder=h264,hevc,aac,mp3,ac3,eac3,flac,opus,vorbis,vp8,vp9,av1,wrapped_avframe",
		"--enable-filter=null,anull,aresample,aformat", "--extra-cflags=-Os -ffunction-sections -fdata-sections",
		"--extra-ldflags=-static -static-libgcc -Wl,--gc-sections",
	}
	if _, err := os.Stat(filepath.Join(work, "Makefile")); err == nil {
		if err := run(work, nil, "make", "distclean"); err != nil {
			return err
		}
	}
	if err := run(work, nil, filepath.Join(source, "configure"), configure...); err != nil {
		return err
	}
	if err := run(work, nil, "make", "-j", strconv.Itoa(runtime.NumCPU())); err != nil {
		return err
	}
	binary := filepath.Join(work, "ffmpeg")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := copyFile(binary, filepath.Join(output, "ffmpeg-slim"+filepath.Ext(binary))); err != nil {
		return err
	}
	return nil
}
func FFmpegAndroid(root, source, output string) error {
	if source == "" {
		return errors.New("ffmpeg-android requires --source")
	}
	_, _, ndk, err := androidEnvironment()
	if err != nil {
		return err
	}
	if output == "" {
		output = filepath.Join(root, ".build", "artifacts", "android", "ffmpeg")
	}
	host := "linux-x86_64"
	if runtime.GOOS == "darwin" {
		host = "darwin-x86_64"
	}
	toolchain := filepath.Join(ndk, "toolchains", "llvm", "prebuilt", host, "bin")
	abis := strings.Split(os.Getenv("ANDROID_ABIS"), ",")
	if len(abis) == 1 && abis[0] == "" {
		abis = []string{"arm64-v8a"}
	}
	api := os.Getenv("ANDROID_API")
	if api == "" {
		api = "24"
	}
	for _, abi := range abis {
		abi = strings.TrimSpace(abi)
		arch := "aarch64"
		triple := "aarch64-linux-android"
		extra := ""
		switch abi {
		case "arm64-v8a":
		case "armeabi-v7a":
			arch = "arm"
			triple = "armv7a-linux-androideabi"
			extra = " -march=armv7-a -mfloat-abi=softfp -mfpu=neon"
		case "x86_64":
			arch = "x86_64"
			triple = "x86_64-linux-android"
		default:
			return fmt.Errorf("unsupported Android ABI %s", abi)
		}
		work := filepath.Join(output, "build", abi)
		prefix := filepath.Join(output, "prefix", abi)
		if err := os.RemoveAll(work); err != nil {
			return err
		}
		if err := ensureDir(work); err != nil {
			return err
		}
		args := []string{"--prefix=" + prefix, "--target-os=android", "--arch=" + arch, "--enable-cross-compile",
			"--cc=" + filepath.Join(toolchain, triple+api+"-clang"),
			"--cxx=" + filepath.Join(toolchain, triple+api+"-clang++"),
			"--ar=" + filepath.Join(toolchain, "llvm-ar"), "--nm=" + filepath.Join(toolchain, "llvm-nm"),
			"--ranlib=" + filepath.Join(toolchain, "llvm-ranlib"), "--strip=" + filepath.Join(toolchain, "llvm-strip"),
			"--extra-cflags=-fPIC -Os" + extra, "--enable-shared", "--disable-static", "--disable-programs",
			"--disable-doc", "--disable-debug", "--disable-autodetect", "--disable-network",
			"--disable-avdevice", "--disable-postproc", "--disable-swscale", "--disable-swresample",
			"--disable-everything", "--enable-demuxer=hls,mpegts,mov,aac", "--enable-muxer=mp4",
			"--enable-protocol=file,crypto,data", "--enable-parser=aac,h264,hevc",
			"--enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb"}
		if err := run(work, nil, filepath.Join(source, "configure"), args...); err != nil {
			return err
		}
		if err := run(work, nil, "make", "-j", strconv.Itoa(runtime.NumCPU())); err != nil {
			return err
		}
		if err := run(work, nil, "make", "install"); err != nil {
			return err
		}
		matches, err := filepath.Glob(filepath.Join(prefix, "lib", "*.so"))
		if err != nil {
			return err
		}
		for _, src := range matches {
			if err := copyFile(src, filepath.Join(output, "jniLibs", abi, filepath.Base(src))); err != nil {
				return err
			}
		}
	}
	return nil
}

// Icons derives the supported sizes from the checked-in master PNG. Generation
// uses the Go standard image codecs instead of a PowerShell/System.Drawing shim.
func Icons(root string) error {
	base := filepath.Join(root, "extension", "icons")
	file, err := os.Open(filepath.Join(base, "icon-128.png"))
	if err != nil {
		return err
	}
	source, err := png.Decode(file)
	file.Close()
	if err != nil {
		return err
	}
	for _, size := range []int{16, 32, 48, 128} {
		dst := image.NewNRGBA(image.Rect(0, 0, size, size))
		bounds := source.Bounds()
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				sx := bounds.Min.X + x*bounds.Dx()/size
				sy := bounds.Min.Y + y*bounds.Dy()/size
				dst.Set(x, y, source.At(sx, sy))
			}
		}
		out, err := os.Create(filepath.Join(base, fmt.Sprintf("icon-%d.png", size)))
		if err != nil {
			return err
		}
		err = png.Encode(out, dst)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}
