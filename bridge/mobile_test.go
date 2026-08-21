package downkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type testPlatformMuxer struct {
	called    bool
	requestID string
}

func (m *testPlatformMuxer) Mux(requestID, _, _, _, output string) error {
	m.called = true
	m.requestID = requestID
	return os.WriteFile(output, []byte("muxed"), 0o644)
}

func TestDownloadMobileRejectsInvalidTask(t *testing.T) {
	if err := DownloadMobile("not-json", nil); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if err := DownloadMobile(`{"url":"https://example.com/a.m3u8"}`, nil); err == nil {
		t.Fatal("expected outputDir error")
	}
}

func TestSafeMobileRequestID(t *testing.T) {
	if got := safeMobileRequestID("request-123_abc"); got != "request-123_abc" {
		t.Fatalf("unexpected request id: %q", got)
	}
	for _, value := range []string{"", "contains spaces", "https://example.com", string(make([]byte, 65))} {
		if got := safeMobileRequestID(value); got != "unknown" {
			t.Fatalf("expected %q to be rejected, got %q", value, got)
		}
	}
}

func TestDownloadMobileHLSUsesPlatformMuxer(t *testing.T) {
	segment := make([]byte, 188*3)
	segment[0], segment[188], segment[376] = 0x47, 0x47, 0x47
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/video.m3u8" {
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nseg.ts\n#EXT-X-ENDLIST\n"))
			return
		}
		_, _ = w.Write(segment)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	task, _ := json.Marshal(map[string]any{
		"url": server.URL + "/video.m3u8", "title": "mobile-test",
		"requestId": "request-123", "outputDir": outputDir, "quality": "best", "concurrent": 2,
	})
	muxer := &testPlatformMuxer{}
	if err := DownloadMobile(string(task), muxer); err != nil {
		t.Fatal(err)
	}
	if !muxer.called {
		t.Fatal("platform muxer was not called")
	}
	if muxer.requestID != "request-123" {
		t.Fatalf("platform muxer request id = %q", muxer.requestID)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "mobile-test.mp4"))
	if err != nil || string(data) != "muxed" {
		t.Fatalf("unexpected output: data=%q err=%v", data, err)
	}
}
