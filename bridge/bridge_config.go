package downkit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const bridgeVersion = "1.0.8"

type bridgeConfig struct {
	OutputDir    string `json:"outputDir"`
	Proxy        string `json:"proxy,omitempty"`
	ProxyEnabled *bool  `json:"proxyEnabled,omitempty"`
	ProxyHost    string `json:"proxyHost"`
	ProxyPort    int    `json:"proxyPort"`
	Concurrent   int    `json:"concurrent"`
	Quality      string `json:"quality"`
	FFmpegPath   string `json:"ffmpegPath"`
	YTDLPPath    string `json:"ytDlpPath"`
}

type bridgeToolStatus struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

type bridgeEnvironment struct {
	Version    string           `json:"version"`
	PID        int              `json:"pid"`
	Platform   string           `json:"platform"`
	Executable string           `json:"executable"`
	FFmpeg     bridgeToolStatus `json:"ffmpeg"`
	YTDLP      bridgeToolStatus `json:"ytDlp"`
}

func bridgeDataPath(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "DownKit", name), nil
}

func defaultBridgeConfig() bridgeConfig {
	home, _ := os.UserHomeDir()
	proxyEnabled := false
	return bridgeConfig{
		OutputDir:    filepath.Join(home, "Downloads"),
		ProxyEnabled: &proxyEnabled,
		Concurrent:   12,
	}
}

func proxyEnabled(config bridgeConfig) bool {
	if config.ProxyEnabled != nil {
		return *config.ProxyEnabled
	}
	return strings.TrimSpace(config.Proxy) != "" || strings.TrimSpace(config.ProxyHost) != "" || config.ProxyPort != 0
}

func normalizeBridgeConfig(config bridgeConfig) (bridgeConfig, error) {
	config.OutputDir = strings.TrimSpace(config.OutputDir)
	config.Proxy = strings.TrimSpace(config.Proxy)
	enabled := proxyEnabled(config)
	var err error
	config.Proxy, config.ProxyHost, config.ProxyPort, err = normalizeProxyAddress(config.Proxy, config.ProxyHost, config.ProxyPort)
	if err != nil {
		return config, err
	}
	config.ProxyEnabled = &enabled
	if !enabled {
		config.Proxy = ""
	} else if config.Proxy == "" {
		return config, errors.New("启用代理前请填写代理主机和端口")
	}
	config.Quality = strings.ToLower(strings.TrimSpace(config.Quality))
	config.FFmpegPath = strings.TrimSpace(config.FFmpegPath)
	config.YTDLPPath = strings.TrimSpace(config.YTDLPPath)
	if config.OutputDir == "" {
		return config, errors.New("下载目录不能为空")
	}
	if config.Concurrent < 1 || config.Concurrent > 64 {
		return config, errors.New("并发数必须在 1 到 64 之间")
	}
	if config.Quality != "" {
		var quality options
		if err := setQuality(&quality, config.Quality); err != nil {
			return config, err
		}
	}
	return config, nil
}

func loadBridgeConfig() bridgeConfig {
	defaults := defaultBridgeConfig()
	path, err := bridgeDataPath("config.json")
	if err != nil {
		return defaults
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	var config bridgeConfig
	if json.Unmarshal(data, &config) != nil {
		return defaults
	}
	if config.OutputDir == "" {
		config.OutputDir = defaults.OutputDir
	}
	if config.Concurrent == 0 {
		config.Concurrent = defaults.Concurrent
	}
	if normalized, err := normalizeBridgeConfig(config); err == nil {
		return normalized
	}
	return defaults
}

func saveBridgeConfig(config bridgeConfig) error {
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		return err
	}
	path, err := bridgeDataPath("config.json")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func inspectTool(preferred, name string) bridgeToolStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return inspectToolContext(ctx, preferred, name)
}

func inspectToolContext(ctx context.Context, preferred, name string) bridgeToolStatus {
	path, err := findTool(preferred, name)
	if err != nil {
		return bridgeToolStatus{Error: err.Error()}
	}
	argument := "-version"
	if name == "yt-dlp" {
		argument = "--version"
	}
	output, err := exec.CommandContext(ctx, path, argument).CombinedOutput()
	if err != nil {
		return bridgeToolStatus{Path: path, Error: strings.TrimSpace(string(output))}
	}
	version := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	return bridgeToolStatus{Available: true, Path: path, Version: version}
}

func inspectBridgeEnvironment(config bridgeConfig) bridgeEnvironment {
	executable, _ := currentExecutable()
	return bridgeEnvironment{
		Version: bridgeVersion, PID: os.Getpid(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Executable: executable,
		FFmpeg:     inspectTool(config.FFmpegPath, "ffmpeg"),
		YTDLP:      inspectTool(config.YTDLPPath, "yt-dlp"),
	}
}
