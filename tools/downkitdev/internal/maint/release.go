package maint

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type target struct{ OS, Name, Arch string }

func targets() []target {
	return []target{{"windows", "windows", "amd64"}, {"linux", "linux", "amd64"}, {"linux", "linux", "arm64"}, {"darwin", "macos", "amd64"}, {"darwin", "macos", "arm64"}}
}

func Package(root, requested string, allowMissing bool, source string) error {
	current, err := version(root)
	if err != nil {
		return err
	}
	if requested != "" && strings.TrimPrefix(requested, "v") != current {
		return fmt.Errorf("version mismatch: requested=%s extension=%s", requested, current)
	}
	code, err := os.ReadFile(filepath.Join(root, "bridge", "bridge_config.go"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(code), `const bridgeVersion = "`+current+`"`) {
		return fmt.Errorf("Bridge version does not match extension %s", current)
	}
	output := filepath.Join(root, ".build", "artifacts", "release", "v"+current)
	if err := ensureDir(output); err != nil {
		return err
	}
	for _, t := range targets() {
		name := fmt.Sprintf("downkit-v%s-%s-%s", current, t.Name, t.Arch)
		stage := filepath.Join(root, ".build", "out", "release-stage", name)
		if err := os.RemoveAll(stage); err != nil {
			return err
		}
		if err := ensureDir(filepath.Join(stage, "bin")); err != nil {
			return err
		}
		ext := ""
		if t.OS == "windows" {
			ext = ".exe"
		}
		if err := Build(root, t.OS, t.Arch, filepath.Join(stage, "bin", "downkit-"+t.Name+"-"+t.Arch+ext)); err != nil {
			return err
		}
		maintOutput := filepath.Join(stage, "bin", "downkitdev"+ext)
		if err := run(filepath.Join(root, "tools", "downkitdev"), []string{"CGO_ENABLED=0", "GOOS=" + t.OS, "GOARCH=" + t.Arch},
			"go", "build", "-trimpath", "-o", maintOutput, "."); err != nil {
			return err
		}
		for _, filename := range []string{"README.md", "README_ZH.md", "LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"} {
			if err := copyFile(filepath.Join(root, filename), filepath.Join(stage, filename)); err != nil {
				return err
			}
		}
		if err := copyTree(filepath.Join(root, "extension"), filepath.Join(stage, "extension"), func(rel string, isDir bool) bool {
			return rel != "test" && !strings.HasPrefix(rel, "test/") && rel != ".DS_Store"
		}); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(root, "docs"), filepath.Join(stage, "docs"), nil); err != nil {
			return err
		}
		if t.OS == "windows" {
			tools := filepath.Join(root, "dist", "tools")
			if _, err := os.Stat(tools); err != nil {
				if !allowMissing {
					return fmt.Errorf("official Windows tools missing in %s (use --allow-missing-tools for incomplete development bundle)", tools)
				}
			} else if err := copyTree(tools, filepath.Join(stage, "tools"), nil); err != nil {
				return err
			}
			for _, required := range []string{"ffmpeg-slim.exe", "whisper/whisper-cli.exe", "llama/llama-server.exe", "COPYING.LGPLv2.1", "ffmpeg-slim-SOURCE.txt"} {
				if _, err := os.Stat(filepath.Join(stage, "tools", filepath.FromSlash(required))); err != nil && !allowMissing {
					return fmt.Errorf("incomplete Windows release: missing licensed component %s", required)
				}
			}
		}
		if err := Components(stage, t.Name, t.Arch, filepath.Join(stage, "components.json")); err != nil {
			return err
		}
		if t.OS == "windows" {
			if err := zipDirectory(stage, filepath.Join(output, name+".zip"), name); err != nil {
				return err
			}
		} else {
			if err := tarGzipDirectory(stage, filepath.Join(output, name+".tar.gz"), name); err != nil {
				return err
			}
		}
		fmt.Println("Packaged:", name)
	}
	// Official releases include the corresponding source archive for the
	// bundled Windows FFmpeg binary. Incomplete development bundles may omit it.
	if source == "" {
		source = filepath.Join(root, ".build", "cache", "source", "FFmpeg-n8.1.2-source.tar.gz")
	}
	if _, err := os.Stat(source); err != nil {
		if !allowMissing {
			return fmt.Errorf("FFmpeg source archive required at %s: %w", source, err)
		}
		source = ""
	}
	if source != "" {
		expected := "9fd092511605bbebafe095ea6d38d9e40f34d12f7386e1258372df8be0576eb7"
		hash, err := digest(source)
		if err != nil {
			return err
		}
		if hash != expected {
			return fmt.Errorf("FFmpeg source SHA mismatch: %s", hash)
		}
		if err := copyFile(source, filepath.Join(output, "FFmpeg-n8.1.2-source.tar.gz")); err != nil {
			return err
		}
	}
	if err := shaManifest(output); err != nil {
		return err
	}
	fmt.Println("Release directory:", output)
	return nil
}

func zipDirectory(root, target, prefix string) error {
	if err := ensureDir(filepath.Dir(target)); err != nil {
		return err
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	out := zip.NewWriter(file)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, relative))
		header.Method = zip.Deflate
		writer, err := out.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, in)
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		return err
	})
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
func tarGzipDirectory(root, target, prefix string) error {
	if err := ensureDir(filepath.Dir(target)); err != nil {
		return err
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	compressor, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		_ = file.Close()
		return err
	}
	compressor.Name = ""
	compressor.ModTime = time.Unix(0, 0)
	writer := tar.NewWriter(compressor)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, relative))
		header.Uid = 0
		header.Gid = 0
		header.Uname = ""
		header.Gname = ""
		if strings.HasPrefix(filepath.ToSlash(relative), "bin/") {
			header.Mode = 0o755
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, in)
		closeErr := in.Close()
		if err == nil {
			err = closeErr
		}
		return err
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := compressor.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

type artifact struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func artifactOf(root, path string) *artifact {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil || info.IsDir() {
		return nil
	}
	hash, err := digest(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return nil
	}
	return &artifact{path, info.Size(), hash}
}
func componentFiles(root, name string) []artifact {
	dir := filepath.Join(root, "tools", name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []artifact
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if a := artifactOf(root, filepath.ToSlash(filepath.Join("tools", name, entry.Name()))); a != nil {
			files = append(files, *a)
		}
	}
	return files
}
func Components(root, platform, arch, output string) error {
	ver, err := version(root)
	if err != nil {
		return err
	}
	if output == "" {
		output = filepath.Join(root, "components.json")
	}
	suffix := ""
	if platform == "windows" {
		suffix = ".exe"
	}
	ffmpeg := artifactOf(root, "tools/ffmpeg-slim"+suffix)
	whisper := artifactOf(root, "tools/whisper/whisper-cli"+suffix)
	llama := artifactOf(root, "tools/llama/llama-server"+suffix)
	delivery := func(a *artifact) string {
		if a != nil {
			return "bundled-sidecar"
		}
		return "external"
	}
	components := []any{
		map[string]any{"id": "ffmpeg", "displayName": "FFmpeg Slim", "delivery": delivery(ffmpeg), "requiredFor": []string{"media.remux", "media.merge"}, "artifact": ffmpeg, "license": "LGPL-2.1-or-later", "licenseFile": "tools/COPYING.LGPLv2.1", "sourceFile": "tools/ffmpeg-slim-SOURCE.txt"},
		map[string]any{"id": "yt-dlp", "displayName": "yt-dlp", "delivery": "on-demand", "requiredFor": []string{"page.resolve", "playlist.resolve"}, "releaseBaseUrl": "https://github.com/yt-dlp/yt-dlp/releases/latest/download", "checksumAsset": "SHA2-256SUMS"},
		map[string]any{"id": "whisper.cpp", "displayName": "whisper.cpp", "delivery": delivery(whisper), "requiredFor": []string{"subtitle.asr", "audio.transcribe"}, "artifact": whisper, "bundleArtifacts": componentFiles(root, "whisper"), "license": "MIT", "licenseFile": "tools/whisper/LICENSE", "sourceFile": "tools/whisper/SOURCE.txt"},
		map[string]any{"id": "llama.cpp", "displayName": "llama.cpp", "delivery": delivery(llama), "requiredFor": []string{"subtitle.translate"}, "artifact": llama, "bundleArtifacts": componentFiles(root, "llama"), "license": "MIT", "licenseFile": "tools/llama/LICENSE", "sourceFile": "tools/llama/SOURCE.txt"},
	}
	return writeJSON(output, map[string]any{"schemaVersion": 1, "productVersion": ver, "platform": platform, "architecture": arch, "generatedAt": time.Now().UTC().Format(time.RFC3339), "components": components})
}
