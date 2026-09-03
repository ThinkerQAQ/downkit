package downkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWhisperModelCatalogMatchesSupportedModels(t *testing.T) {
	if len(whisperModelCatalog) != 30 {
		t.Fatalf("catalog size = %d; want 30", len(whisperModelCatalog))
	}
	seen := make(map[string]bool, len(whisperModelCatalog))
	for _, model := range whisperModelCatalog {
		if model.ID == "" || model.Size <= 0 || len(model.SHA256) != sha256.Size*2 {
			t.Fatalf("invalid model descriptor: %#v", model)
		}
		if seen[model.ID] {
			t.Fatalf("duplicate model %q", model.ID)
		}
		seen[model.ID] = true
	}
	for _, id := range []string{"tiny", "base", "small-q5_1", "medium", "large-v3-turbo-q8_0", "small.en-tdrz"} {
		if !seen[id] {
			t.Fatalf("catalog is missing %q", id)
		}
	}
}

func TestWhisperModelViewsMarkConfiguredModelActive(t *testing.T) {
	model, ok := findWhisperModel("base")
	if !ok {
		t.Fatal("base model missing")
	}
	path := filepath.Join(t.TempDir(), model.filename())
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(model.Size); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	views := whisperModelViews(bridgeConfig{WhisperModel: path})
	for _, view := range views {
		if view.ID == "base" {
			if !view.Installed || !view.Active || view.Path != path || !view.Recommended {
				t.Fatalf("unexpected base view: %#v", view)
			}
			return
		}
	}
	t.Fatal("base view missing")
}

func TestDownloadWhisperModelResumesAndVerifies(t *testing.T) {
	payload := []byte(strings.Repeat("model-data-", 4096))
	digest := sha256.Sum256(payload)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		start := 0
		if value := request.Header.Get("Range"); strings.HasPrefix(value, "bytes=") {
			parsed, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "bytes="), "-"))
			if err != nil {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			start = parsed
			response.WriteHeader(http.StatusPartialContent)
		}
		_, _ = response.Write(payload[start:])
	}))
	defer server.Close()

	model := whisperModelDescriptor{
		ID: "test", Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:]), BaseURL: server.URL,
	}
	target := filepath.Join(t.TempDir(), model.filename())
	partialSize := len(payload) / 3
	if err := os.WriteFile(target+".part", payload[:partialSize], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := downloadWhisperModel(context.Background(), server.Client(), model, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("unexpected model download: bytes=%d err=%v", len(data), err)
	}
}

func TestDownloadWhisperModelRejectsHashMismatch(t *testing.T) {
	payload := []byte("tampered-model")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	model := whisperModelDescriptor{ID: "test", Size: int64(len(payload)), SHA256: strings.Repeat("a", 64), BaseURL: server.URL}
	target := filepath.Join(t.TempDir(), model.filename())
	if err := downloadWhisperModel(context.Background(), server.Client(), model, target); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unverified model was activated: %v", err)
	}
}

func TestHandleInstallWhisperModelRejectsUnknownModel(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: "secret"}, config: defaultBridgeConfig()}
	request := httptest.NewRequest(http.MethodPost, "/v1/tools/whisper/models/install", strings.NewReader(`{"model":"unknown"}`))
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()
	server.handleInstallWhisperModel(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}
