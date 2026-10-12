package maint

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const ffmpegTag = "n8.1.2"
const ffmpegSourceSHA = "9fd092511605bbebafe095ea6d38d9e40f34d12f7386e1258372df8be0576eb7"

func FFmpegWindows(root, source, output, tag string) error {
	if runtime.GOOS != "windows" {
		return errors.New("ffmpeg Windows builder must run on Windows with MSYS2")
	}
	if tag == "" {
		tag = ffmpegTag
	}
	if tag != ffmpegTag {
		return fmt.Errorf("unverified FFmpeg release tag %q (pinned version %s)", tag, ffmpegTag)
	}
	msys := `C:\msys64\usr\bin\bash.exe`
	if _, err := os.Stat(msys); err != nil {
		msys = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "msys64", "usr", "bin", "bash.exe")
		if _, err := os.Stat(msys); err != nil {
			return errors.New("MSYS2 is required: install winget MSYS2.MSYS2")
		}
	}
	env := []string{"CHERE_INVOKING=1", "MSYSTEM=UCRT64"}
	if err := run(root, env, msys, "-lc", "pacman -S --needed --noconfirm make diffutils curl pkgconf mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-nasm"); err != nil {
		return err
	}
	cache := filepath.Join(root, ".build", "cache", "ffmpeg", tag)
	if err := ensureDir(cache); err != nil {
		return err
	}
	if source == "" {
		source = filepath.Join(cache, "ffmpeg")
		if _, err := os.Stat(filepath.Join(source, "configure")); os.IsNotExist(err) {
			archive := filepath.Join(cache, "FFmpeg-"+tag+".tar.gz")
			if _, err := os.Stat(archive); os.IsNotExist(err) {
				url := "https://github.com/FFmpeg/FFmpeg/archive/refs/tags/" + tag + ".tar.gz"
				client := &http.Client{Timeout: 12 * time.Minute}
				request, err := http.NewRequest("GET", url, nil)
				if err != nil {
					return err
				}
				response, err := client.Do(request)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("FFmpeg source download HTTP %d", response.StatusCode)
				}
				tmp := archive + ".partial"
				file, err := os.Create(tmp)
				if err != nil {
					return err
				}
				_, err = io.Copy(file, io.LimitReader(response.Body, 350*1024*1024))
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
				if err != nil {
					return err
				}
				hash, err := digest(tmp)
				if err != nil {
					return err
				}
				if hash != ffmpegSourceSHA {
					return fmt.Errorf("FFmpeg archive SHA mismatch: %s", hash)
				}
				if err := os.Rename(tmp, archive); err != nil {
					return err
				}
			}
			hash, err := digest(archive)
			if err != nil {
				return err
			}
			if hash != ffmpegSourceSHA {
				return fmt.Errorf("FFmpeg archive SHA mismatch: %s", hash)
			}
			if err := extractTarGzipStripOne(archive, source); err != nil {
				return err
			}
		}
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(source, "configure")); err != nil {
		return fmt.Errorf("FFmpeg configure missing: %w", err)
	}
	if output == "" {
		output = filepath.Join(root, "dist", "tools")
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := ensureDir(output); err != nil {
		return err
	}
	work := filepath.Join(cache, "native")
	if err := ensureDir(work); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte(tag+"\n"), 0o644); err != nil {
		return err
	}
	sourceUnix, err := msysPath(msys, source, env)
	if err != nil {
		return err
	}
	workUnix, err := msysPath(msys, work, env)
	if err != nil {
		return err
	}
	outputUnix, err := msysPath(msys, output, env)
	if err != nil {
		return err
	}
	args := []string{
		"--disable-everything", "--disable-autodetect", "--disable-network", "--disable-doc", "--disable-debug",
		"--disable-programs", "--enable-ffmpeg", "--enable-small", "--enable-static", "--disable-shared",
		"--enable-protocol=file,pipe,crypto,data",
		"--enable-demuxer=hls,mpegts,mov,matroska,webm,aac,mp3,ac3,eac3,ogg,flac,wav",
		"--enable-muxer=mp4,mov,matroska,webm,adts,wav", "--enable-encoder=pcm_s16le",
		"--enable-parser=h264,hevc,av1,vp8,vp9,aac,ac3,opus,vorbis",
		"--enable-bsf=aac_adtstoasc,extract_extradata,h264_mp4toannexb,hevc_mp4toannexb,vp9_superframe",
		"--enable-decoder=h264,hevc,aac,mp3,ac3,eac3,flac,opus,vorbis,vp8,vp9,av1,wrapped_avframe",
		"--enable-filter=null,anull,aresample,aformat", "--extra-cflags=-Os -ffunction-sections -fdata-sections",
		"--extra-ldflags=-static -static-libgcc -Wl,--gc-sections",
	}
	escaped := make([]string, len(args))
	for index, arg := range args {
		escaped[index] = bashQuote(arg)
	}
	commands := []string{
		"export PATH=/ucrt64/bin:/usr/bin",
		"cd " + bashQuote(workUnix),
		"if test -f Makefile; then make distclean; fi",
		bashQuote(sourceUnix+"/configure") + " " + strings.Join(escaped, " "),
		"make -j4",
		"strip ffmpeg.exe 2>/dev/null || true",
		"cp -f ffmpeg.exe " + bashQuote(outputUnix+"/ffmpeg-slim.exe"),
	}
	if err := run(root, env, msys, "-lc", strings.Join(commands, " && ")); err != nil {
		return err
	}
	return run(output, nil, filepath.Join(output, "ffmpeg-slim.exe"), "-hide_banner", "-version")
}
func bashQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
func msysPath(bash, path string, env []string) (string, error) {
	cmd := exec.Command(bash, "-lc", `cygpath -u "$1"`, "_", path)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cygpath %s: %w", path, err)
	}
	return strings.TrimSpace(string(out)), nil
}
func extractTarGzipStripOne(archive, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := ensureDir(destination); err != nil {
		return err
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gzipReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(header.Name), "/")
		if len(parts) < 2 {
			continue
		}
		rel := filepath.Join(parts[1:]...)
		cleaned := filepath.Clean(rel)
		if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return fmt.Errorf("untrusted tar entry %s", header.Name)
		}
		dest := filepath.Join(destination, cleaned)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensureDir(dest); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := ensureDir(filepath.Dir(dest)); err != nil {
				return err
			}
			output, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, err = io.Copy(output, io.LimitReader(tarReader, 256*1024*1024))
			closeErr := output.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
