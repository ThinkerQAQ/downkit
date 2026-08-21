package downkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fixedManagedTool struct{ item toolSnapshot }

func (t fixedManagedTool) Name() string { return t.item.Name }
func (t fixedManagedTool) Snapshot(_ context.Context, _ bridgeConfig) toolSnapshot {
	return t.item
}

func TestToolRegistryOrdersAndDeduplicatesTools(t *testing.T) {
	registry := newToolRegistry(
		fixedManagedTool{toolSnapshot{Name: "later", SortOrder: 20}},
		fixedManagedTool{toolSnapshot{Name: "first", SortOrder: 10}},
		fixedManagedTool{toolSnapshot{Name: "later", DisplayName: "replacement", SortOrder: 15}},
	)
	items := registry.snapshots(context.Background(), bridgeConfig{})
	if len(items) != 2 || items[0].Name != "first" || items[1].DisplayName != "replacement" {
		t.Fatalf("unexpected registry snapshot: %#v", items)
	}
}

func TestDesktopToolsOwnTheirConfigSchema(t *testing.T) {
	config := defaultBridgeConfig()
	config.FFmpegPath = "custom-ffmpeg"
	config.YTDLPPath = "custom-yt-dlp"
	items := newDesktopToolRegistry().snapshots(context.Background(), config)
	byName := make(map[string]toolSnapshot, len(items))
	for _, item := range items {
		byName[item.Name] = item
	}
	if byName["bridge"].Config.Values["address"] != bridgeBaseURL {
		t.Fatalf("bridge does not own its address config: %#v", byName["bridge"].Config)
	}
	if byName["go-downloader"].Config.Values["outputDir"] != config.OutputDir {
		t.Fatalf("downloader config missing: %#v", byName["go-downloader"].Config)
	}
	if byName["go-downloader"].Kind != "capability" {
		t.Fatalf("downloader kind = %q", byName["go-downloader"].Kind)
	}
	for _, item := range items {
		if item.Config.DefaultExpanded {
			t.Fatalf("tool %q should be collapsed by default", item.Name)
		}
		encoded, err := json.Marshal(item.Config)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), `"defaultExpanded":false`) {
			t.Fatalf("tool %q config does not publish its collapsed default: %s", item.Name, encoded)
		}
	}
	if len(byName["go-downloader"].Actions) != 1 || byName["go-downloader"].Actions[0].ID != "open-output" {
		t.Fatalf("missing downloader open-output action: %#v", byName["go-downloader"].Actions)
	}
	if byName["bridge"].Kind != "runtime" {
		t.Fatalf("bridge kind = %q", byName["bridge"].Kind)
	}
	if len(byName["bridge"].Actions) != 1 || byName["bridge"].Actions[0].ID != "restart" {
		t.Fatalf("missing bridge restart action: %#v", byName["bridge"].Actions)
	}
	if byName["ffmpeg"].Config.Values["ffmpegPath"] != "custom-ffmpeg" {
		t.Fatalf("ffmpeg config missing: %#v", byName["ffmpeg"].Config)
	}
	if byName["yt-dlp"].Delivery != "on-demand" {
		t.Fatalf("yt-dlp delivery = %q", byName["yt-dlp"].Delivery)
	}
	if len(byName["yt-dlp"].Actions) != 1 || byName["yt-dlp"].Actions[0].ID != "install" {
		t.Fatalf("missing yt-dlp install action: %#v", byName["yt-dlp"].Actions)
	}
	if byName["network-proxy"].Config.Values["proxyHost"] != config.ProxyHost {
		t.Fatalf("proxy config missing: %#v", byName["network-proxy"].Config)
	}
	if byName["network-proxy"].Config.Toggle == nil || byName["network-proxy"].Config.Toggle.Key != "proxyEnabled" {
		t.Fatalf("proxy toggle missing: %#v", byName["network-proxy"].Config)
	}
	wantKeys := map[string]bool{"address": true, "outputDir": true, "proxyHost": true, "proxyPort": true, "concurrent": true, "quality": true, "ffmpegPath": true, "ytDlpPath": true}
	for _, item := range items {
		for _, field := range item.Config.Schema {
			if !wantKeys[field.Key] {
				t.Fatalf("unexpected config key %q on %s", field.Key, item.Name)
			}
			delete(wantKeys, field.Key)
		}
	}
	if len(wantKeys) != 0 {
		t.Fatalf("config keys are not owned by tools: %#v", wantKeys)
	}
}

func TestMobileToolsJSONUsesPlatformToolsWithoutBridge(t *testing.T) {
	var payload struct {
		Version int            `json:"version"`
		Tools   []toolSnapshot `json:"tools"`
	}
	if err := json.Unmarshal([]byte(MobileToolsJSON()), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || len(payload.Tools) != 3 {
		t.Fatalf("unexpected mobile payload: %#v", payload)
	}
	for _, item := range payload.Tools {
		if item.Name == "bridge" || item.Name == "native-host" {
			t.Fatalf("desktop-only tool leaked to mobile: %#v", item)
		}
	}
	if payload.Tools[2].Name != "ffmpeg" || payload.Tools[2].Delivery != "bundled-native" || payload.Tools[2].Health.Status != "unsupported" {
		t.Fatalf("unexpected mobile ffmpeg descriptor: %#v", payload.Tools[2])
	}
}

func TestHandleToolsReturnsRegistrySnapshot(t *testing.T) {
	server := &bridgeServer{
		state:  bridgeState{Token: "secret"},
		config: defaultBridgeConfig(),
		tools: newToolRegistry(fixedManagedTool{toolSnapshot{
			Name: "test", DisplayName: "Test Tool", SortOrder: 1,
			Health: toolHealth{Status: "ready", OK: true},
		}}),
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()
	server.handleTools(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		OK    bool           `json:"ok"`
		Tools []toolSnapshot `json:"tools"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || len(payload.Tools) != 1 || payload.Tools[0].Name != "test" {
		t.Fatalf("unexpected tools response: %#v", payload)
	}
}

func TestHandleToolsRejectsUnauthorizedRequest(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: "secret"}}
	request := httptest.NewRequest(http.MethodGet, "/v1/tools", nil)
	response := httptest.NewRecorder()
	server.handleTools(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestHandleRestartRejectsUnfinishedJob(t *testing.T) {
	server := &bridgeServer{
		state: bridgeState{Token: "secret"},
		jobs: map[string]*bridgeJob{
			"running": {ID: "running", Status: "running"},
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/restart", nil)
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()
	server.handleRestart(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}

func TestHandleRestartRejectsUnavailableRestart(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: "secret"}, jobs: make(map[string]*bridgeJob)}
	request := httptest.NewRequest(http.MethodPost, "/v1/restart", nil)
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()
	server.handleRestart(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}
