package downkit

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurlCompatibilityArgumentsPreserveTaskContext(t *testing.T) {
	a := &app{
		opts: options{
			sourceURL: "https://media.test/master.m3u8",
			proxy:     "http://127.0.0.1:8080",
			referer:   "https://page.test/watch",
			origin:    "https://page.test",
			requestHeaders: map[string]string{
				"Accept-Language": "zh-CN",
			},
			mediaCookies: []browserCookie{{Name: "session", Value: "test", Domain: "media.test", HostOnly: true, Path: "/", Secure: true}},
		},
		workDir: t.TempDir(),
	}
	args, err := a.curlBaseArgs(a.opts.sourceURL, filepath.Join(a.workDir, "out"), true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	for _, expected := range []string{
		"--proxy\nhttp://127.0.0.1:8080",
		"--header\nOrigin: https://page.test",
		"--header\nReferer: https://page.test/watch",
		"--header\nAccept-Language: zh-CN",
		"--cookie\n" + filepath.Join(a.workDir, "curl-cookies.txt"),
		"--url\nhttps://media.test/master.m3u8",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing curl argument pair %q in %q", expected, joined)
		}
	}
	if strings.Contains(joined, "--header\nCookie:") {
		t.Fatalf("cookie must use curl's scoped cookie engine: %q", joined)
	}
}

func TestCurlCompatibilityFetchesPlaylistAndRange(t *testing.T) {
	if _, err := curlExecutable(); err != nil {
		t.Skip(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nsegment.ts\n#EXT-X-ENDLIST\n"))
		case "/segment.ts":
			if r.Header.Get("Range") != "bytes=2-5" {
				t.Errorf("unexpected range: %q", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 2-5/8")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("2345"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	a := &app{opts: options{sourceURL: server.URL + "/master.m3u8"}, workDir: dir}
	body, status, err := a.fetchHTTPCurl(a.opts.sourceURL)
	if err != nil || status != http.StatusOK || !strings.HasPrefix(string(body), "#EXTM3U") {
		t.Fatalf("curl playlist fetch failed: status=%d body=%q err=%v", status, body, err)
	}
	d, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	item := segment{
		url: server.URL + "/segment.ts", name: "range",
		path: filepath.Join(dir, "segment.bin"), rangeStart: 2, rangeLength: 4,
	}
	if err := d.downloadSegmentWithCurl(item); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, item.path, []byte("2345"))
}
