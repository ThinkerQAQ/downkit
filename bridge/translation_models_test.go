package downkit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranslationModelCatalogPinsRecommendedGGUF(t *testing.T) {
	if len(translationModelCatalog) != 1 {
		t.Fatalf("translation model catalog size = %d", len(translationModelCatalog))
	}
	model := translationModelCatalog[0]
	if model.ID != "translategemma-4b-q4-k-m" || model.Size != 2489909312 || len(model.SHA256) != 64 || !strings.HasPrefix(model.URL, "https://huggingface.co/") {
		t.Fatalf("unexpected translation model: %#v", model)
	}
	view := translationModelViews(bridgeConfig{})[0]
	if !view.Recommended || !view.RequiresLicenseAcceptance || view.License != "Gemma" || view.LicenseURL != gemmaTermsURL {
		t.Fatalf("translation model license metadata missing: %#v", view)
	}
}

func TestDownloadTranslationModelVerifiesPayload(t *testing.T) {
	payload := []byte("GGUF-test-model")
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write(payload)
	}))
	defer server.Close()
	model := translationModelDescriptor{ID: "test", Filename: "test.gguf", Size: int64(len(payload)), SHA256: digest, URL: server.URL}
	target := filepath.Join(t.TempDir(), model.Filename)
	if err := downloadTranslationModel(context.Background(), server.Client(), model, target); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != string(payload) {
		t.Fatalf("downloaded model mismatch: %q, %v", data, err)
	}
}

func TestInstallTranslationModelRequiresLicenseAcceptance(t *testing.T) {
	if _, err := installTranslationModelComponent(context.Background(), "", translationModelCatalog[0].ID, false, bridgeConfig{}); err == nil || !strings.Contains(err.Error(), "接受 Gemma") {
		t.Fatalf("missing license acceptance was not rejected: %v", err)
	}
}

func TestHandleInstallTranslationModelRejectsMissingAcceptance(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: "secret"}, config: defaultBridgeConfig()}
	request := httptest.NewRequest(http.MethodPost, "/v1/tools/llama/models/install", strings.NewReader(`{"model":"translategemma-4b-q4-k-m"}`))
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()
	server.handleInstallTranslationModel(response, request)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "接受 Gemma") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}
