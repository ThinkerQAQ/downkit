package maint

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type releaseTool struct {
	name, tag, archive, sha, entry string
	required                       []string
}

var pinnedTools = map[string]releaseTool{
	"whisper": {"whisper", "b4938", "whisper-bin-x64.zip", "c2a4b60edb11f7e11a9191ffb50929535527d4d91c9903dbe3e554583bbbc63d", "Release", []string{"whisper-cli.exe", "whisper.dll", "ggml.dll", "ggml-base.dll"}},
	"llama":   {"llama", "b10621", "llama-b10621-bin-win-cpu-x64.zip", "0e8b65e650e369f70f8307d890508886f171ef4fb00facccddd4a1b7ffdaca51", "", []string{"llama-server.exe", "llama-server-impl.dll", "llama-common.dll", "llama.dll", "ggml.dll", "ggml-base.dll", "libomp.dll", "mtmd.dll", "LICENSE-LLVM-OpenMP"}},
}

func InstallTool(root, name, provided string) error {
	value, ok := pinnedTools[name]
	if !ok {
		return fmt.Errorf("unknown verified tool %q (use whisper or llama)", name)
	}
	if name == "whisper" {
		value.name = "whisper.cpp"
	} else {
		value.name = "llama.cpp"
	}
	cached := filepath.Join(root, ".devtool", "cache", "components", name, value.tag, value.archive)
	if err := ensureDir(filepath.Dir(cached)); err != nil {
		return err
	}
	archive := provided
	if archive == "" {
		archive = cached
		if _, err := os.Stat(archive); os.IsNotExist(err) {
			address := "https://github.com/ggml-org/" + value.name + "/releases/download/" + value.tag + "/" + value.archive
			client := &http.Client{Timeout: 12 * time.Minute}
			request, err := http.NewRequest(http.MethodGet, address, nil)
			if err != nil {
				return err
			}
			request.Header.Set("User-Agent", "DownKit-Go-Maintenance")
			response, err := client.Do(request)
			if err != nil {
				return err
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("download %s: HTTP %d", address, response.StatusCode)
			}
			tmp := archive + ".partial"
			file, err := os.Create(tmp)
			if err != nil {
				return err
			}
			_, err = io.Copy(file, io.LimitReader(response.Body, 1024*1024*1024))
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(tmp)
				return err
			}
			hash, err := digest(tmp)
			if err != nil {
				return err
			}
			if hash != value.sha {
				_ = os.Remove(tmp)
				return fmt.Errorf("%s official archive hash mismatch", name)
			}
			if err := os.Rename(tmp, archive); err != nil {
				return err
			}
		}
	}
	hash, err := digest(archive)
	if err != nil {
		return err
	}
	if hash != value.sha {
		return fmt.Errorf("%s SHA-256 mismatch: expected %s, got %s", name, value.sha, hash)
	}
	input, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer input.Close()
	stage := filepath.Join(root, ".devtool", "cache", "components", name, value.tag, "stage")
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	if err := ensureDir(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	selected := map[string]bool{}
	for _, z := range input.File {
		basename := filepath.Base(z.Name)
		needed := false
		for _, required := range value.required {
			if basename == required {
				needed = true
			}
		}
		if strings.HasPrefix(basename, "ggml-cpu-") && strings.HasSuffix(basename, ".dll") {
			needed = true
		}
		if !needed {
			continue
		}
		clean := filepath.ToSlash(filepath.Clean(z.Name))
		if strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			return fmt.Errorf("invalid archive path %q", z.Name)
		}
		// Reject duplicate basenames, which otherwise overwrite trusted binaries.
		if selected[basename] {
			return fmt.Errorf("duplicate file in upstream archive: %s", basename)
		}
		selected[basename] = true
		in, err := z.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(stage, basename), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(in, 512*1024*1024))
		closeOut := out.Close()
		closeIn := in.Close()
		if err == nil {
			err = closeOut
		}
		if err == nil {
			err = closeIn
		}
		if err != nil {
			return err
		}
	}
	for _, required := range value.required {
		if !selected[required] {
			return fmt.Errorf("official archive missing required component %s", required)
		}
	}
	hasCPU := false
	for key := range selected {
		if strings.HasPrefix(key, "ggml-cpu-") {
			hasCPU = true
		}
	}
	if !hasCPU {
		return errors.New("upstream release does not contain a CPU backend")
	}
	for _, filename := range []string{"LICENSE", "SOURCE.txt"} {
		if err := copyFile(filepath.Join(root, "build", name+"-cpp", filename), filepath.Join(stage, filename)); err != nil {
			return err
		}
	}
	executable := "llama-server.exe"
	if name == "whisper" {
		executable = "whisper-cli.exe"
	}
	if err := run(stage, nil, filepath.Join(stage, executable), "--version"); err != nil {
		return fmt.Errorf("staged %s failed validation: %w", name, err)
	}
	dest := filepath.Join(root, "dist", "tools", name)
	backup := dest + ".previous"
	if err := ensureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dest); err != nil {
		if _, exist := os.Stat(backup); exist == nil {
			_ = os.Rename(backup, dest)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	fmt.Println(name, "ready:", dest)
	return nil
}
