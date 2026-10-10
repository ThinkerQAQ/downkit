package downkit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsoleServesSharedExtensionAssets(t *testing.T) {
	server := &bridgeServer{}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/console/popup/popup.html", nil)
	response := httptest.NewRecorder()
	server.handleConsole(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET console status=%d, expected 200 (body=%s)", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "popup.css") {
		t.Fatal("console must serve the shared popup UI")
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("console must set CSP")
	}
}
func TestConsoleRejectsForeignHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://malicious.example/console/", nil)
	rec := httptest.NewRecorder()
	(&bridgeServer{}).handleConsole(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, expected 403", rec.Code)
	}
}
func TestConsoleRejectsMutation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17891/console/", nil)
	rec := httptest.NewRecorder()
	(&bridgeServer{}).handleConsole(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, expected 405", rec.Code)
	}
}
func TestConsoleRejectsCrossSiteResourceFetch(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17891/console/popup/core.js", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	rec := httptest.NewRecorder()
	(&bridgeServer{}).handleConsole(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d, expected 403", rec.Code)
	}
}
