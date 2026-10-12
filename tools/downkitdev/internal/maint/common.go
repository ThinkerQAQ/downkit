package maint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type extensionManifest struct {
	Version string `json:"version"`
}

func FindRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root, ok := findRootFrom(cwd); ok {
		return root, nil
	}
	exe, err := os.Executable()
	if err == nil {
		if root, ok := findRootFrom(filepath.Dir(exe)); ok {
			return root, nil
		}
	}
	return "", errors.New("DownKit root not found (expected extension/manifest.json); use --root")
}
func findRootFrom(dir string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "extension", "manifest.json")); err == nil {
			return dir, true
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
	}
}
func version(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "extension", "manifest.json"))
	if err != nil {
		return "", err
	}
	var manifest extensionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", err
	}
	if manifest.Version == "" {
		return "", errors.New("missing extension version")
	}
	return manifest.Version, nil
}
func run(dir string, env []string, command string, args ...string) error {
	fmt.Println("+", command, strings.Join(args, " "))
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return nil
}
func tool(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}
func ensureDir(path string) error { return os.MkdirAll(path, 0o755) }
func copyFile(from, to string) error {
	f, err := os.Open(from)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(to)); err != nil {
		return err
	}
	tmp := to + ".part"
	o, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, err = io.Copy(o, f)
	if closeErr := o.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, to); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func copyTree(from, to string, filter func(relative string, isDir bool) bool) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return ensureDir(to)
		}
		if filter != nil && !filter(filepath.ToSlash(rel), d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return ensureDir(filepath.Join(to, rel))
		}
		return copyFile(path, filepath.Join(to, rel))
	})
}
func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func writeJSON(path string, value any) error {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o600)
}
func verificationTargets(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(root, "extension"), func(path string, dir fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !dir.IsDir() && strings.HasSuffix(dir.Name(), ".js") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}
func Verify(root string) error {
	if _, err := os.Stat(filepath.Join(root, "bridge", "go.mod")); err != nil {
		return fmt.Errorf("source workspace required for verify: %w", err)
	}
	if err := run(filepath.Join(root, "bridge"), nil, "go", "test", "./..."); err != nil {
		return err
	}
	if err := run(filepath.Join(root, "tools", "downkitdev"), nil, "go", "test", "./..."); err != nil {
		return err
	}
	scripts, err := verificationTargets(root)
	if err != nil {
		return err
	}
	for _, file := range scripts {
		if err := run(root, nil, "node", "--check", file); err != nil {
			return err
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "extension", "test", "*.test.js"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		if err := run(root, nil, "node", file); err != nil {
			return err
		}
	}
	fmt.Println("Go and browser checks passed")
	return nil
}
func Build(root, targetOS, targetArch, output string) error {
	if _, err := os.Stat(filepath.Join(root, "bridge", "go.mod")); err != nil {
		return errors.New("build requires the source checkout")
	}
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	if output == "" {
		suffix := ""
		if targetOS == "windows" {
			suffix = ".exe"
		}
		output = filepath.Join(root, ".build", "out", fmt.Sprintf("downkit-%s-%s%s", targetOS, targetArch, suffix))
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(absolute)); err != nil {
		return err
	}
	env := []string{"CGO_ENABLED=0", "GOOS=" + targetOS, "GOARCH=" + targetArch}
	if err := run(filepath.Join(root, "bridge"), env, "go", "build", "-buildvcs=false", "-trimpath", "-o", absolute, "./cmd/downkit"); err != nil {
		return err
	}
	fmt.Println("Bridge:", absolute)
	return nil
}
func shaManifest(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	var lines []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "SHA256SUMS.txt" {
			continue
		}
		value, err := digest(filepath.Join(directory, entry.Name()))
		if err != nil {
			return err
		}
		lines = append(lines, value+"  "+entry.Name())
	}
	sort.Strings(lines)
	return os.WriteFile(filepath.Join(directory, "SHA256SUMS.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
