package downkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type sessionTestMuxer struct {
	called bool
}

func (m *sessionTestMuxer) Mux(request muxRequest) error {
	m.called = true
	return os.WriteFile(request.Output, []byte("muxed"), 0o644)
}

func TestHLSTaskKeepsBrowserAndRotatedSession(t *testing.T) {
	segmentData := make([]byte, 188*3)
	segmentData[0], segmentData[188], segmentData[376] = 0x47, 0x47, 0x47
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browserCookie, _ := r.Cookie("browser")
		if browserCookie == nil || browserCookie.Value != "seed" {
			http.Error(w, "browser session required", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/master.m3u8":
			http.SetCookie(w, &http.Cookie{Name: "rotated", Value: "ready", Path: "/"})
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000,RESOLUTION=640x360\nvariant.m3u8\n"))
		case "/variant.m3u8":
			rotatedCookie, _ := r.Cookie("rotated")
			if rotatedCookie == nil || rotatedCookie.Value != "ready" {
				http.Error(w, "rotated session required", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nsegment.ts\n#EXT-X-ENDLIST\n"))
		case "/segment.ts":
			rotatedCookie, _ := r.Cookie("rotated")
			if rotatedCookie == nil || rotatedCookie.Value != "ready" {
				http.Error(w, "rotated session required", http.StatusForbidden)
				return
			}
			_, _ = w.Write(segmentData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outputDir := t.TempDir()
	muxer := &sessionTestMuxer{}
	err := runWithOptions(options{
		sourceURL:    server.URL + "/master.m3u8",
		title:        "session-e2e",
		outputDir:    outputDir,
		qualitySet:   true,
		concurrent:   2,
		playlistMode: "single",
		mediaCookies: []browserCookie{{Name: "browser", Value: "seed", Domain: "127.0.0.1", HostOnly: true, Path: "/"}},
	}, muxer)
	if err != nil {
		t.Fatal(err)
	}
	if !muxer.called {
		t.Fatal("muxer was not called")
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "session-e2e.mp4"))
	if err != nil || string(data) != "muxed" {
		t.Fatalf("unexpected output: data=%q err=%v", data, err)
	}
}

func TestHLSTaskUsesConfiguredProxyAsPrimaryRoute(t *testing.T) {
	segmentData := make([]byte, 188*3)
	segmentData[0], segmentData[188], segmentData[376] = 0x47, 0x47, 0x47
	directHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directHits++
		http.Error(w, "direct route must not be used", http.StatusForbidden)
	}))
	defer target.Close()

	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		switch r.URL.Path {
		case "/master.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nsegment.ts\n#EXT-X-ENDLIST\n"))
		case "/segment.ts":
			_, _ = w.Write(segmentData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer proxy.Close()

	outputDir := t.TempDir()
	muxer := &sessionTestMuxer{}
	err := runWithOptions(options{
		sourceURL:    target.URL + "/master.m3u8",
		title:        "proxy-primary-e2e",
		proxy:        proxy.URL,
		outputDir:    outputDir,
		qualitySet:   true,
		concurrent:   2,
		playlistMode: "single",
	}, muxer)
	if err != nil {
		t.Fatal(err)
	}
	if directHits != 0 {
		t.Fatalf("configured proxy was bypassed: direct hits=%d", directHits)
	}
	if proxyHits < 2 {
		t.Fatalf("playlist and segment should use the configured proxy: proxy hits=%d", proxyHits)
	}
}
