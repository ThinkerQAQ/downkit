package downkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const bridgeVersion = "1.2.1"

type bridgeConfig struct {
	OutputDir        string `json:"outputDir"`
	Proxy            string `json:"proxy,omitempty"`
	ProxyEnabled     *bool  `json:"proxyEnabled,omitempty"`
	ProxyHost        string `json:"proxyHost"`
	ProxyPort        int    `json:"proxyPort"`
	Concurrent       int    `json:"concurrent"`
	Quality          string `json:"quality"`
	FFmpegPath       string `json:"ffmpegPath"`
	YTDLPPath        string `json:"ytDlpPath"`
	WhisperPath      string `json:"whisperPath"`
	WhisperModel     string `json:"whisperModel"`
	LlamaPath        string `json:"llamaPath"`
	TranslationModel string `json:"translationModel"`
	OpenMode         string `json:"openMode"`
	UILanguage       string `json:"uiLanguage"`
	ShowLogs         bool   `json:"showLogs"`
	DetectHls        bool   `json:"detectHls"`
	DetectDash       bool   `json:"detectDash"`
	DetectMp4        bool   `json:"detectMp4"`
	LogEnabled       bool   `json:"logEnabled"`
	LogMaxMB         int    `json:"logMaxMB"`
}

type fileConfig struct {
	Download struct {
		OutputDir  string `toml:"output_dir"`
		Concurrent int    `toml:"concurrent"`
		Quality    string `toml:"quality"`
	} `toml:"download"`
	Proxy struct {
		Enabled bool   `toml:"enabled"`
		URL     string `toml:"url"`
		Host    string `toml:"host"`
		Port    int    `toml:"port"`
	} `toml:"proxy"`
	Tools struct {
		FFmpeg           string `toml:"ffmpeg"`
		YTDLP            string `toml:"yt_dlp"`
		Whisper          string `toml:"whisper"`
		WhisperModel     string `toml:"whisper_model"`
		Llama            string `toml:"llama"`
		TranslationModel string `toml:"translation_model"`
	} `toml:"tools"`
	Interface struct {
		OpenMode string `toml:"open_mode"`
		Language string `toml:"language"`
		ShowLogs *bool  `toml:"show_logs,omitempty"`
	} `toml:"interface"`
	Sniffer struct {
		HLS  bool `toml:"hls"`
		DASH bool `toml:"dash"`
		MP4  bool `toml:"mp4"`
	} `toml:"sniffer"`
	Logging struct {
		Enabled bool `toml:"enabled"`
		MaxMB   int  `toml:"max_size_mb"`
	} `toml:"logging"`
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

func defaultBridgeConfig() bridgeConfig {
	dir, err := bridgeDataPath("downloads")
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Downloads")
	}
	disabled := false
	return bridgeConfig{
		OutputDir: dir, Concurrent: 12, ProxyEnabled: &disabled,
		OpenMode: "popup", UILanguage: "zh-CN", ShowLogs: true,
		DetectHls: true, DetectDash: true, DetectMp4: true,
		LogEnabled: true, LogMaxMB: 10,
	}
}

func proxyEnabled(c bridgeConfig) bool {
	if c.ProxyEnabled != nil {
		return *c.ProxyEnabled
	}
	return strings.TrimSpace(c.Proxy) != "" || strings.TrimSpace(c.ProxyHost) != "" || c.ProxyPort != 0
}

func normalizeBridgeConfig(c bridgeConfig) (bridgeConfig, error) {
	c.OutputDir = strings.TrimSpace(c.OutputDir)
	c.Proxy = strings.TrimSpace(c.Proxy)
	enabled := proxyEnabled(c)
	var err error
	c.Proxy, c.ProxyHost, c.ProxyPort, err = normalizeProxyAddress(c.Proxy, c.ProxyHost, c.ProxyPort)
	if err != nil {
		return c, err
	}
	c.ProxyEnabled = &enabled
	if !enabled {
		c.Proxy = ""
	} else if c.Proxy == "" {
		return c, errors.New("启用代理前请填写代理主机和端口")
	}
	c.Quality = strings.ToLower(strings.TrimSpace(c.Quality))
	c.FFmpegPath = strings.TrimSpace(c.FFmpegPath)
	c.YTDLPPath = strings.TrimSpace(c.YTDLPPath)
	c.WhisperPath = strings.TrimSpace(c.WhisperPath)
	c.WhisperModel = strings.TrimSpace(c.WhisperModel)
	c.LlamaPath = strings.TrimSpace(c.LlamaPath)
	c.TranslationModel = strings.TrimSpace(c.TranslationModel)
	if c.OutputDir == "" {
		return c, errors.New("下载目录不能为空")
	}
	if c.Concurrent < 1 || c.Concurrent > 64 {
		return c, errors.New("并发数必须在 1 到 64 之间")
	}
	if c.Quality != "" {
		var quality options
		if err := setQuality(&quality, c.Quality); err != nil {
			return c, err
		}
	}
	if c.OpenMode != "popup" && c.OpenMode != "sidePanel" {
		return c, errors.New("invalid open mode")
	}
	if c.UILanguage != "en" && c.UILanguage != "zh-CN" {
		return c, errors.New("invalid interface language")
	}
	if !c.DetectHls && !c.DetectDash && !c.DetectMp4 {
		return c, errors.New("至少保留一种媒体检测类型")
	}
	if c.LogMaxMB < 1 || c.LogMaxMB > 100 {
		return c, errors.New("log max size must be 1–100 MB")
	}
	return c, nil
}

func configToFile(c bridgeConfig) fileConfig {
	var f fileConfig
	f.Download.OutputDir = c.OutputDir
	f.Download.Concurrent = c.Concurrent
	f.Download.Quality = c.Quality
	f.Proxy.Enabled = proxyEnabled(c)
	f.Proxy.URL = c.Proxy
	f.Proxy.Host = c.ProxyHost
	f.Proxy.Port = c.ProxyPort
	f.Tools.FFmpeg = c.FFmpegPath
	f.Tools.YTDLP = c.YTDLPPath
	f.Tools.Whisper = c.WhisperPath
	f.Tools.WhisperModel = c.WhisperModel
	f.Tools.Llama = c.LlamaPath
	f.Tools.TranslationModel = c.TranslationModel
	f.Interface.OpenMode = c.OpenMode
	f.Interface.Language = c.UILanguage
	f.Interface.ShowLogs = &c.ShowLogs
	f.Sniffer.HLS = c.DetectHls
	f.Sniffer.DASH = c.DetectDash
	f.Sniffer.MP4 = c.DetectMp4
	f.Logging.Enabled = c.LogEnabled
	f.Logging.MaxMB = c.LogMaxMB
	return f
}
func configFromFile(f fileConfig) bridgeConfig {
	showLogs := true
	if f.Interface.ShowLogs != nil {
		showLogs = *f.Interface.ShowLogs
	}
	return bridgeConfig{
		OutputDir: f.Download.OutputDir, Concurrent: f.Download.Concurrent, Quality: f.Download.Quality,
		Proxy: f.Proxy.URL, ProxyEnabled: &f.Proxy.Enabled, ProxyHost: f.Proxy.Host, ProxyPort: f.Proxy.Port,
		FFmpegPath: f.Tools.FFmpeg, YTDLPPath: f.Tools.YTDLP,
		WhisperPath: f.Tools.Whisper, WhisperModel: f.Tools.WhisperModel,
		LlamaPath: f.Tools.Llama, TranslationModel: f.Tools.TranslationModel,
		OpenMode: f.Interface.OpenMode, UILanguage: f.Interface.Language, ShowLogs: showLogs,
		DetectHls: f.Sniffer.HLS, DetectDash: f.Sniffer.DASH, DetectMp4: f.Sniffer.MP4,
		LogEnabled: f.Logging.Enabled, LogMaxMB: f.Logging.MaxMB,
	}
}

func loadBridgeConfig() bridgeConfig {
	defaults := defaultBridgeConfig()
	path, err := bridgeDataPath("config.toml")
	if err != nil {
		return defaults
	}
	if data, err := os.ReadFile(path); err == nil {
		var f fileConfig
		if toml.Unmarshal(data, &f) == nil {
			if c, err := normalizeBridgeConfig(configFromFile(f)); err == nil {
				return c
			}
		}
		// A malformed TOML file is not overwritten with defaults.
		return defaults
	}
	_ = saveBridgeConfig(defaults)
	return defaults
}

func saveBridgeConfig(c bridgeConfig) error {
	normalized, err := normalizeBridgeConfig(c)
	if err != nil {
		return err
	}
	path, err := bridgeDataPath("config.toml")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := toml.Marshal(configToFile(normalized))
	if err != nil {
		return fmt.Errorf("encode TOML: %w", err)
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
