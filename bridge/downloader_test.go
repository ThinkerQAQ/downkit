package downkit

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadSegmentBatchRetriesAndRanges(t *testing.T) {
	var flakyCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://page.example/" || r.Header.Get("Origin") != "https://page.example" {
			t.Errorf("missing access headers: Referer=%q Origin=%q", r.Header.Get("Referer"), r.Header.Get("Origin"))
		}
		switch r.URL.Path {
		case "/flaky":
			if flakyCalls.Add(1) == 1 {
				http.Error(w, "retry", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("segment-one"))
		case "/range":
			if r.Header.Get("Range") != "bytes=4-9" {
				t.Errorf("unexpected Range: %q", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 4-9/16")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("456789"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	a := &app{opts: options{
		concurrent: 4,
		referer:    "https://page.example/",
		origin:     "https://page.example",
	}}
	d, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	segments := []segment{
		{url: server.URL + "/flaky", name: "one", path: filepath.Join(dir, "one.ts")},
		{url: server.URL + "/range", name: "two", path: filepath.Join(dir, "two.m4s"), rangeStart: 4, rangeLength: 6},
	}
	if err := d.downloadSegmentBatch(segments); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, segments[0].path, []byte("segment-one"))
	assertFileContent(t, segments[1].path, []byte("456789"))
}

func TestInterruptedHLSSegmentFallsBackFromRejectedRange(t *testing.T) {
	content := []byte("complete-segment")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Range") != "" {
			http.Error(w, "range forbidden", http.StatusForbidden)
			return
		}
		_, _ = w.Write(content)
	}))
	defer server.Close()

	dir := t.TempDir()
	item := segment{url: server.URL + "/segment", name: "segment.ts", path: filepath.Join(dir, "segment.ts")}
	if err := os.WriteFile(item.path+".part", []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{opts: options{concurrent: 1}}
	d, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	if err := d.downloadSegment(item); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want ranged request plus full retry", calls.Load())
	}
	assertFileContent(t, item.path, content)
}

func TestSegmentProgressDoesNotCountFailuresAsCompleted(t *testing.T) {
	all := []segment{
		{name: "complete", path: filepath.Join(t.TempDir(), "complete.ts")},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing.ts")},
	}
	if err := os.WriteFile(all[0].path, []byte("complete"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := segmentCompletionPercent(all); got != 50 {
		t.Fatalf("completion = %d, want 50", got)
	}
}

func TestGoDownloaderKeepsProtocolNegotiationAndSeparatePools(t *testing.T) {
	a := &app{opts: options{concurrent: 12}}
	first, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	second, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()

	if first.transport.Protocols != nil {
		t.Fatalf("media protocols should be negotiated automatically: %v", first.transport.Protocols)
	}
	if !first.transport.ForceAttemptHTTP2 {
		t.Fatal("media transport should retain HTTP/2 negotiation")
	}
	if first.transport == second.transport {
		t.Fatal("workers must use separate connection pools")
	}
}

func TestDirectPartCountUsesSharedConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name       string
		size       int64
		configured int
		want       int
	}{
		{name: "single connection is respected", size: 1 << 30, configured: 1, want: 1},
		{name: "large file uses configured limit", size: 1 << 30, configured: 16, want: 16},
		{name: "small file avoids excessive ranges", size: 20 << 20, configured: 16, want: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := directPartCount(test.size, test.configured); got != test.want {
				t.Fatalf("directPartCount(%d, %d) = %d, want %d", test.size, test.configured, got, test.want)
			}
		})
	}
}

func TestDownloadRangedFileAndResumeStream(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 64*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Range"), "bytes=")
		pieces := strings.SplitN(raw, "-", 2)
		if len(pieces) != 2 {
			_, _ = w.Write(data)
			return
		}
		start, _ := strconv.ParseInt(pieces[0], 10, 64)
		end := int64(len(data) - 1)
		if pieces[1] != "" {
			end, _ = strconv.ParseInt(pieces[1], 10, 64)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	defer server.Close()

	a := &app{opts: options{concurrent: 4}}
	d, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	dir := t.TempDir()
	rangedOutput := filepath.Join(dir, "ranged.mp4")
	info := remoteFileInfo{size: int64(len(data)), rangeSupport: true, etag: "test"}
	if err := d.downloadRangedFile(server.URL, rangedOutput, info, 4); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, rangedOutput, data)

	streamOutput := filepath.Join(dir, "stream.mp4")
	if err := os.WriteFile(streamOutput+".part", data[:12345], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.downloadStreamFile(server.URL, streamOutput, info); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, streamOutput, data)
}

func assertFileContent(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("content mismatch for %s: got %d bytes, want %d", path, len(actual), len(expected))
	}
}
