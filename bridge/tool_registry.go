package downkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type toolConfigField struct {
	Key         string             `json:"key"`
	Label       string             `json:"label"`
	Type        string             `json:"type"`
	Default     any                `json:"default,omitempty"`
	Min         *int               `json:"min,omitempty"`
	Max         *int               `json:"max,omitempty"`
	Placeholder string             `json:"placeholder,omitempty"`
	Description string             `json:"description,omitempty"`
	ReadOnly    bool               `json:"readOnly,omitempty"`
	Advanced    bool               `json:"advanced,omitempty"`
	Options     []toolConfigOption `json:"options,omitempty"`
}

type toolConfigOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type toolConfigView struct {
	Schema          []toolConfigField `json:"schema"`
	Values          map[string]any    `json:"values"`
	Toggle          *toolConfigToggle `json:"toggle,omitempty"`
	DefaultExpanded bool              `json:"defaultExpanded"`
}

type toolConfigToggle struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type toolHealth struct {
	Status    string    `json:"status"`
	OK        bool      `json:"ok"`
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail,omitempty"`
	Version   string    `json:"version,omitempty"`
	Path      string    `json:"path,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

type toolSnapshot struct {
	Name         string         `json:"name"`
	DisplayName  string         `json:"displayName"`
	Kind         string         `json:"kind"`
	Description  string         `json:"description,omitempty"`
	Platforms    []string       `json:"platforms"`
	Delivery     string         `json:"delivery"`
	Required     bool           `json:"required"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Actions      []toolAction   `json:"actions,omitempty"`
	Config       toolConfigView `json:"config"`
	Health       toolHealth     `json:"health"`
	SortOrder    int            `json:"sortOrder"`
}

type toolAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// managedTool is the small cross-platform management contract. A tool owns
// its identity, configuration metadata and health probe; jobs remain runtime
// executions and are deliberately not represented as tools.
type managedTool interface {
	Name() string
	Snapshot(context.Context, bridgeConfig) toolSnapshot
}

type toolRegistry struct {
	tools []managedTool
}

func newToolRegistry(tools ...managedTool) *toolRegistry {
	byName := make(map[string]managedTool, len(tools))
	for _, tool := range tools {
		if tool != nil && strings.TrimSpace(tool.Name()) != "" {
			byName[tool.Name()] = tool
		}
	}
	registered := make([]managedTool, 0, len(byName))
	for _, tool := range byName {
		registered = append(registered, tool)
	}
	return &toolRegistry{tools: registered}
}

func (r *toolRegistry) snapshots(ctx context.Context, config bridgeConfig) []toolSnapshot {
	if r == nil {
		return nil
	}
	items := make([]toolSnapshot, len(r.tools))
	var group sync.WaitGroup
	for index, tool := range r.tools {
		group.Add(1)
		go func() {
			defer group.Done()
			items[index] = tool.Snapshot(ctx, config)
		}()
	}
	group.Wait()
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].SortOrder != items[right].SortOrder {
			return items[left].SortOrder < items[right].SortOrder
		}
		return items[left].Name < items[right].Name
	})
	return items
}

type bridgeRuntimeTool struct{}

func (bridgeRuntimeTool) Name() string { return "bridge" }

func (bridgeRuntimeTool) Snapshot(_ context.Context, _ bridgeConfig) toolSnapshot {
	executable, _ := currentExecutable()
	return toolSnapshot{
		Name: "bridge", DisplayName: "DownKit Bridge", Kind: "runtime",
		Description: "连接浏览器与本地下载引擎的服务。",
		Platforms:   []string{"windows", "linux", "darwin"}, Delivery: "bundled", Required: true,
		Capabilities: []string{"task.submit", "job.manage", "config.persist"}, SortOrder: 20,
		Config: toolConfigView{
			Schema: []toolConfigField{{Key: "address", Label: "监听地址", Type: "string", ReadOnly: true}},
			Values: map[string]any{"address": bridgeBaseURL},
		},
		Health: toolHealth{
			Status: "ready", OK: true, Summary: "Bridge 在线",
			Detail:  fmt.Sprintf("PID %d · %s/%s", os.Getpid(), runtime.GOOS, runtime.GOARCH),
			Version: bridgeVersion, Path: executable, CheckedAt: time.Now(),
		},
		Actions: []toolAction{{
			ID: "restart", Label: "重启 Bridge",
			Description: "重新启动本地服务；存在未结束的下载任务时会拒绝操作。",
		}},
	}
}

type goDownloaderTool struct{}

func (goDownloaderTool) Name() string { return "go-downloader" }

func intPointer(value int) *int { return &value }

func (goDownloaderTool) Snapshot(_ context.Context, config bridgeConfig) toolSnapshot {
	health := toolHealth{Status: "ready", OK: true, Summary: "下载引擎就绪", CheckedAt: time.Now()}
	if _, err := normalizeBridgeConfig(config); err != nil {
		health = toolHealth{Status: "error", OK: false, Summary: "配置不可用", Detail: err.Error(), CheckedAt: time.Now()}
	}
	return toolSnapshot{
		Name: "go-downloader", DisplayName: "Go 下载引擎", Kind: "capability",
		Description: "负责清单解析、分片下载、解密和断点续传。",
		Platforms:   []string{"windows", "linux", "darwin", "android", "ios"}, Delivery: "bundled", Required: true,
		Capabilities: []string{"http.direct", "hls.download", "aes128.decrypt"}, SortOrder: 40,
		Config: toolConfigView{
			Schema: []toolConfigField{
				{Key: "outputDir", Label: "下载目录", Type: "directory"},
				{Key: "concurrent", Label: "下载并发上限", Type: "integer", Default: 12, Min: intPointer(1), Max: intPointer(64), Description: "统一控制 HLS 分片、MP4 Range 和 yt-dlp 分片的最大并发数。"},
				{Key: "quality", Label: "默认清晰度", Type: "select", Description: "仅在媒体清单包含多个码率时生效。", Options: []toolConfigOption{
					{Value: "", Label: "有多个时询问"},
					{Value: "best", Label: "最高"},
					{Value: "1080", Label: "1080p"},
					{Value: "720", Label: "720p"},
					{Value: "480", Label: "480p"},
				}},
			},
			Values: map[string]any{"outputDir": config.OutputDir, "concurrent": config.Concurrent, "quality": config.Quality},
		},
		Health: health,
		Actions: []toolAction{{
			ID: "open-output", Label: "打开下载目录",
			Description: "在文件管理器中打开当前下载目录。",
		}},
	}
}

type networkProxyTool struct{}

func (networkProxyTool) Name() string { return "network-proxy" }

func (networkProxyTool) Snapshot(ctx context.Context, config bridgeConfig) toolSnapshot {
	health := toolHealth{Status: "disabled", OK: true, Summary: "未启用 · Bridge 外网直连", CheckedAt: time.Now()}
	enabled := proxyEnabled(config)
	if enabled && config.Proxy != "" {
		latency, target, err := probeAnyProxyConnectivity(ctx, config.Proxy, proxyConnectivityURLs[:])
		if err != nil {
			health = toolHealth{Status: "error", OK: false, Summary: "代理网络不通", Detail: err.Error(), CheckedAt: time.Now()}
		} else {
			health = toolHealth{Status: "ready", OK: true, Summary: "代理网络通畅", Detail: fmt.Sprintf("外网请求经 %s · %s · %d ms", config.Proxy, logURLSummary(target), latency.Milliseconds()), CheckedAt: time.Now()}
		}
	}
	return toolSnapshot{
		Name: "network-proxy", DisplayName: "网络代理", Kind: "infra",
		Description: "统一配置 Bridge 发起的外网请求；本机控制通信保持直连。",
		Platforms:   []string{"windows", "linux", "darwin"}, Delivery: "bundled", Required: enabled,
		Capabilities: []string{"http.proxy", "network.probe"}, SortOrder: 30,
		Config: toolConfigView{
			Toggle: &toolConfigToggle{Key: "proxyEnabled", Label: "启用代理", Description: "关闭后保留代理地址，Bridge 外网请求改为直连。"},
			Schema: []toolConfigField{
				{Key: "proxyHost", Label: "代理主机", Type: "string", Placeholder: "例如 127.0.0.1", Description: "只填写域名或 IP，不要包含 http:// 或端口。"},
				{Key: "proxyPort", Label: "代理端口", Type: "integer", Min: intPointer(1), Max: intPointer(65535), Description: "主机和端口都留空表示直连。"},
			},
			Values: map[string]any{"proxyEnabled": enabled, "proxyHost": config.ProxyHost, "proxyPort": config.ProxyPort},
		},
		Health: health,
	}
}

type executableTool struct {
	name, displayName, executableName, configKey, description, delivery string
	required, advanced                                                  bool
	capabilities                                                        []string
	sortOrder                                                           int
}

func (t executableTool) Name() string { return t.name }

func (t executableTool) configuredPath(config bridgeConfig) string {
	if t.configKey == "ffmpegPath" {
		return config.FFmpegPath
	}
	return config.YTDLPPath
}

func (t executableTool) Snapshot(ctx context.Context, config bridgeConfig) toolSnapshot {
	configured := t.configuredPath(config)
	status := inspectToolContext(ctx, configured, t.executableName)
	health := toolHealth{Status: "missing", OK: false, Summary: "组件未找到", Detail: status.Error, CheckedAt: time.Now()}
	if status.Available {
		health = toolHealth{
			Status: "ready", OK: true, Summary: "组件就绪", Version: status.Version,
			Path: status.Path, CheckedAt: time.Now(),
		}
		if t.name == "ffmpeg" {
			if version, err := parseFFmpegVersion(status.Version); err != nil {
				health.Status, health.OK, health.Summary, health.Detail = "incompatible", false, "版本无法识别", err.Error()
			} else if err := checkFFmpegMinimum(version); err != nil {
				health.Status, health.OK, health.Summary, health.Detail = "incompatible", false, "版本过低", err.Error()
			}
		}
	}
	if t.name == "ffmpeg" && health.Status == "missing" {
		health.Summary = "随包组件缺失"
		health.Detail = "FFmpeg Slim 应由 DownKit 安装包提供；请修复或重新安装 DownKit，也可在下方指定可信的 FFmpeg 程序。"
	}
	item := toolSnapshot{
		Name: t.name, DisplayName: t.displayName, Kind: "dependency", Description: t.description,
		Platforms: []string{"windows", "linux", "darwin"}, Delivery: t.delivery, Required: t.required,
		Capabilities: append([]string(nil), t.capabilities...), SortOrder: t.sortOrder,
		Config: toolConfigView{
			Schema: []toolConfigField{{
				Key: t.configKey, Label: "程序路径", Type: "file", Advanced: t.advanced,
				Placeholder: "请输入程序完整路径", Description: "自动检测失败时，请指定可信的程序文件。",
			}},
			Values: map[string]any{t.configKey: configured},
		},
		Health: health,
	}
	if t.name == "yt-dlp" && health.Status == "missing" {
		item.Actions = []toolAction{{
			ID: "install", Label: "下载组件",
			Description: "从 yt-dlp 官方 release 下载并校验 SHA-256。",
		}}
	}
	return item
}

func newDesktopToolRegistry() *toolRegistry {
	return newToolRegistry(
		bridgeRuntimeTool{},
		networkProxyTool{},
		goDownloaderTool{},
		executableTool{
			name: "ffmpeg", displayName: "FFmpeg Slim", executableName: "ffmpeg", configKey: "ffmpegPath",
			description: "无损封装 HLS、TS、fMP4 及分离音视频轨。", delivery: "bundled-sidecar",
			capabilities: []string{"media.remux", "media.merge"}, sortOrder: 50, advanced: true,
		},
		executableTool{
			name: "yt-dlp", displayName: "yt-dlp", executableName: "yt-dlp", configKey: "ytDlpPath",
			description: "按需解析来源页面和通用媒体站点。", delivery: "on-demand", required: false,
			capabilities: []string{"page.resolve", "playlist.resolve"}, sortOrder: 60, advanced: true,
		},
	)
}

type mobileStaticTool struct{ snapshot toolSnapshot }

func (t mobileStaticTool) Name() string { return t.snapshot.Name }
func (t mobileStaticTool) Snapshot(_ context.Context, _ bridgeConfig) toolSnapshot {
	t.snapshot.Health.CheckedAt = time.Now()
	return t.snapshot
}

func newMobileToolRegistry() *toolRegistry {
	return newToolRegistry(
		mobileStaticTool{toolSnapshot{
			Name: "downkit-core", DisplayName: "DownKit Core", Kind: "core",
			Description: "通过 gomobile 嵌入的 Go 下载引擎。", Platforms: []string{"android", "ios"},
			Delivery: "bundled", Required: true, Capabilities: []string{"http.direct", "hls.download", "aes128.decrypt"},
			Config: toolConfigView{Schema: []toolConfigField{}, Values: map[string]any{}},
			Health: toolHealth{Status: "ready", OK: true, Summary: "Go 核心就绪", Version: bridgeVersion}, SortOrder: 10,
		}},
		mobileStaticTool{toolSnapshot{
			Name: "platform-muxer", DisplayName: "系统媒体封装器", Kind: "platform",
			Description: "Android MediaExtractor/MediaMuxer 或 iOS 平台实现。", Platforms: []string{"android", "ios"},
			Delivery: "bundled", Required: true, Capabilities: []string{"media.remux"},
			Config: toolConfigView{Schema: []toolConfigField{}, Values: map[string]any{}},
			Health: toolHealth{Status: "ready", OK: true, Summary: "由宿主应用提供"}, SortOrder: 20,
		}},
		mobileStaticTool{toolSnapshot{
			Name: "ffmpeg", DisplayName: "FFmpeg Slim", Kind: "dependency",
			Description: "系统封装失败时使用的原生回退；必须随 APK/AAB 发布。", Platforms: []string{"android", "ios"},
			Delivery: "bundled-native", Required: false, Capabilities: []string{"media.remux", "media.merge"},
			Config: toolConfigView{Schema: []toolConfigField{}, Values: map[string]any{}},
			Health: toolHealth{Status: "unsupported", OK: false, Summary: "JNI/平台封装尚未接入"}, SortOrder: 30,
		}},
	)
}

// MobileToolsJSON exposes the same management contract through gomobile
// without asking Java/Kotlin to bind Go interfaces or map-heavy structs.
func MobileToolsJSON() string {
	tools := newMobileToolRegistry().snapshots(context.Background(), bridgeConfig{})
	data, err := json.Marshal(map[string]any{"version": 1, "tools": tools})
	if err != nil {
		return `{"version":1,"tools":[]}`
	}
	return string(data)
}
