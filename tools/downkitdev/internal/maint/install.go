package maint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const hostName = "com.downkit.bridge"
const oldHost = "com.catchhls.go"

type nativeHost struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func exeForRoot(root string) (string, error) {
	candidates := []string{
		filepath.Join(root, "bin", "downkit-windows-amd64.exe"),
		filepath.Join(root, "bin", "downkit-linux-amd64"),
		filepath.Join(root, "bin", "downkit-linux-arm64"),
		filepath.Join(root, "bin", "downkit-macos-amd64"),
		filepath.Join(root, "bin", "downkit-macos-arm64"),
		filepath.Join(root, "downkit-"+runtime.GOOS+"-"+runtime.GOARCH),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return filepath.Abs(candidate)
		}
	}
	return "", errors.New("installed Bridge binary not found under bin/")
}
func readOrigins(path string, mapOrigins map[string]bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var value nativeHost
	if json.Unmarshal(data, &value) != nil || value.Name != hostName {
		return
	}
	for _, origin := range value.AllowedOrigins {
		if strings.HasPrefix(origin, "chrome-extension://") && strings.HasSuffix(origin, "/") {
			id := strings.TrimSuffix(strings.TrimPrefix(origin, "chrome-extension://"), "/")
			if len(id) == 32 && strings.Trim(id, "abcdefghijklmnop") == "" {
				mapOrigins[origin] = true
			}
		}
	}
}
func regKey(key, value string) error {
	cmd := exec.Command("reg.exe", "add", key, "/ve", "/t", "REG_SZ", "/d", value, "/f")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add %s: %w: %s", key, err, strings.TrimSpace(string(output)))
	}
	return nil
}
func removeReg(key string) error {
	cmd := exec.Command("reg.exe", "delete", key, "/f")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(output)), "unable to find") && !strings.Contains(strings.ToLower(string(output)), "cannot find") {
		return fmt.Errorf("reg delete %s: %w: %s", key, err, string(output))
	}
	return nil
}

func Install(root, extensionID string) error {
	binary, err := exeForRoot(root)
	if err != nil {
		return err
	}
	if len(extensionID) != 32 || strings.Trim(extensionID, "abcdefghijklmnop") != "" {
		return errors.New("expected a 32-character Chromium extension ID, a-p only")
	}
	origins := map[string]bool{"chrome-extension://" + extensionID + "/": true}
	dataRoot := filepath.Join(root, "Data")
	for _, subdir := range []string{"logs", "models", "downloads", "tools", "work", "native-host"} {
		if err := ensureDir(filepath.Join(dataRoot, subdir)); err != nil {
			return err
		}
	}
	manifest := filepath.Join(dataRoot, "native-host", hostName+".json")
	readOrigins(manifest, origins)
	var allowed []string
	for origin := range origins {
		allowed = append(allowed, origin)
	}
	sort.Strings(allowed)
	config := nativeHost{hostName, "DownKit local bridge", binary, "stdio", allowed}
	if err := writeJSON(manifest, config); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		if err := regKey(`HKCU\Software\Classes\downkit`, "URL:DownKit Protocol"); err != nil {
			return err
		}
		if err := regKey(`HKCU\Software\Classes\downkit\shell\open\command`, fmt.Sprintf(`"%s" "%%1"`, binary)); err != nil {
			return err
		}
		// A string-valued URL Protocol property is required for Windows URL registration.
		cmd := exec.Command("reg.exe", "add", `HKCU\Software\Classes\downkit`, "/v", "URL Protocol", "/t", "REG_SZ", "/d", "", "/f")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("URL protocol: %w %s", err, string(out))
		}
		for _, browser := range []string{`Google\Chrome`, `Microsoft\Edge`} {
			if err := regKey(`HKCU\Software\`+browser+`\NativeMessagingHosts\`+hostName, manifest); err != nil {
				return err
			}
			_ = removeReg(`HKCU\Software\` + browser + `\NativeMessagingHosts\` + oldHost)
		}
		_ = removeReg(`HKCU\Software\Classes\catchhlsgo`)
	case "linux", "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		for _, directory := range nativeDirs(home) {
			if err := ensureDir(directory); err != nil {
				return err
			}
			dest := filepath.Join(directory, hostName+".json")
			if err := copyFile(manifest, dest); err != nil {
				return err
			}
			if err := os.Chmod(dest, 0o600); err != nil {
				return err
			}
		}
		if err := os.Chmod(binary, 0o755); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported installer platform: %s", runtime.GOOS)
	}
	fmt.Println("Native Messaging installed:", binary)
	fmt.Println("Host manifest:", manifest)
	return nil
}
func nativeDirs(home string) []string {
	if runtime.GOOS == "darwin" {
		return []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "Chromium", "NativeMessagingHosts"),
			filepath.Join(home, "Library", "Application Support", "Microsoft Edge", "NativeMessagingHosts"),
		}
	}
	return []string{
		filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "chromium", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "microsoft-edge", "NativeMessagingHosts"),
	}
}
func Uninstall(root string) error {
	switch runtime.GOOS {
	case "windows":
		if err := removeReg(`HKCU\Software\Classes\downkit`); err != nil {
			return err
		}
		for _, browser := range []string{`Google\Chrome`, `Microsoft\Edge`} {
			if err := removeReg(`HKCU\Software\` + browser + `\NativeMessagingHosts\` + hostName); err != nil {
				return err
			}
		}
	case "linux", "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		for _, dir := range nativeDirs(home) {
			if err := os.Remove(filepath.Join(dir, hostName+".json")); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	path := filepath.Join(root, "Data", "native-host", hostName+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Println("Unregistered Native Messaging; Data and downloads were preserved")
	return nil
}
func Deploy(root, target, targetOS, targetArch string) error {
	destination, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "bridge", "go.mod")); err != nil {
		return errors.New("deploy requires source checkout")
	}
	if err := ensureDir(filepath.Join(destination, "bin")); err != nil {
		return err
	}
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	ext := ""
	if targetOS == "windows" {
		ext = ".exe"
	}
	platform := targetOS
	if platform == "darwin" {
		platform = "macos"
	}
	buildBridge := filepath.Join(root, ".build", "out", "downkit-"+platform+"-"+targetArch+ext)
	if err := Build(root, targetOS, targetArch, buildBridge); err != nil {
		return err
	}
	// A running Windows executable is locked. Stage it rather than overwriting
	// an active Bridge or risking a partially deployed installation.
	binPath := filepath.Join(destination, "bin", "downkit-"+platform+"-"+targetArch+ext)
	if err := copyFile(buildBridge, binPath); err != nil {
		return fmt.Errorf("cannot replace running binary (%s); stop Bridge, then retry: %w", binPath, err)
	}
	if err := run(filepath.Join(root, "tools", "downkitdev"), []string{"CGO_ENABLED=0", "GOOS=" + targetOS, "GOARCH=" + targetArch}, "go", "build", "-trimpath", "-o", filepath.Join(destination, "bin", "downkitdev"+ext), "."); err != nil {
		return err
	}
	for _, dir := range []string{"extension", "docs"} {
		if err := copyTree(filepath.Join(root, dir), filepath.Join(destination, dir), nil); err != nil {
			return err
		}
	}
	for _, file := range []string{"README.md", "README_ZH.md", "LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"} {
		if err := copyFile(filepath.Join(root, file), filepath.Join(destination, file)); err != nil {
			return err
		}
	}
	if err := ensureDir(filepath.Join(destination, "Data")); err != nil {
		return err
	}
	fmt.Println("Deployed program files; existing Data/ was not modified:", destination)
	return nil
}
