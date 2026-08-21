package downkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type checkpointTestMuxer struct{ called bool }

func (m *checkpointTestMuxer) Mux(request muxRequest) error {
	m.called = true
	return os.WriteFile(request.Output, []byte("muxed"), 0o644)
}

func TestBridgeJobStoreRestoresActiveJobAsPausedWithoutCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	now := time.Now().UTC().Truncate(time.Second)
	jobs := map[string]*bridgeJob{
		"job-1": {
			ID: "job-1", Title: "video", Status: "running", Phase: "downloading",
			Progress: 42, DownloadedBytes: 1024, CreatedAt: now, UpdatedAt: now,
			task: bridgeTask{
				URL: "https://example.test/video.m3u8", MediaHeaders: map[string]string{
					"Authorization": "Bearer secret", "Referer": "https://example.test/",
				},
				MediaCookies:  []browserCookie{{Name: "session", Value: "secret", Domain: "example.test"}},
				CookieStoreID: "profile-1",
			},
		},
	}
	if err := saveBridgeJobs(path, jobs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" {
		t.Fatal("empty job store")
	}
	if strings.Contains(string(data), "Bearer secret") || strings.Contains(string(data), `"value": "secret"`) {
		t.Fatalf("credential leaked into job store: %s", data)
	}
	restored, err := loadBridgeJobs(path)
	if err != nil {
		t.Fatal(err)
	}
	job := restored["job-1"]
	if job == nil || job.Status != "needs-session" || !job.RequiresFreshSession || job.Progress != 42 || job.DownloadedBytes != 1024 {
		t.Fatalf("unexpected restored job: %#v", job)
	}
	if len(job.task.MediaCookies) != 0 || job.task.MediaHeaders["Authorization"] != "" {
		t.Fatalf("credentials were persisted: %#v", job.task)
	}
	if job.task.MediaHeaders["Referer"] == "" {
		t.Fatalf("safe task context was lost: %#v", job.task)
	}
	if job.task.CookieStoreID != "profile-1" {
		t.Fatalf("cookie store identity was lost: %#v", job.task)
	}
}

func TestHLSResumePlanRestoresStableSegmentPaths(t *testing.T) {
	workDir := t.TempDir()
	a := &app{
		workDir: workDir, segmentDir: filepath.Join(workDir, "segments"),
		localPlaylist: filepath.Join(workDir, "local.m3u8"),
	}
	if err := os.MkdirAll(a.segmentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.localPlaylist, []byte("#EXTM3U"), 0o600); err != nil {
		t.Fatal(err)
	}
	segments := []segment{{url: "https://cdn.test/seg.ts", name: "seg000000.ts", path: filepath.Join(a.segmentDir, "seg000000.ts")}}
	if err := saveHLSResumePlan(a, segments, nil, ""); err != nil {
		t.Fatal(err)
	}
	plan, ok := loadHLSResumePlan(a)
	if !ok {
		t.Fatal("resume plan was not loaded")
	}
	restored := restoreSegments(plan.Segments, a.segmentDir)
	if len(restored) != 1 || restored[0].path != segments[0].path || restored[0].url != segments[0].url {
		t.Fatalf("unexpected restored segments: %#v", restored)
	}
}

func TestHLSResumeRefreshesSessionAndDownloadsOnlyMissingSegment(t *testing.T) {
	segmentData := make([]byte, 188*3)
	segmentData[0], segmentData[188], segmentData[376] = 0x47, 0x47, 0x47
	playlistRequests := 0
	segmentRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/video.m3u8" {
			playlistRequests++
			_, _ = response.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
			return
		}
		segmentRequests++
		_, _ = response.Write(segmentData)
	}))
	defer server.Close()

	outputDir := t.TempDir()
	workDir := filepath.Join(outputDir, ".downkit-work", "job_resume-test")
	segmentDir := filepath.Join(workDir, "segments")
	if err := os.MkdirAll(segmentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localPlaylist := "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:1,\nsegments/seg000000.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(filepath.Join(workDir, "local.m3u8"), []byte(localPlaylist), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &app{workDir: workDir, segmentDir: segmentDir, localPlaylist: filepath.Join(workDir, "local.m3u8")}
	segments := []segment{{url: server.URL + "/seg.ts", name: "seg000000.ts", path: filepath.Join(segmentDir, "seg000000.ts")}}
	if err := saveHLSResumePlan(a, segments, nil, ""); err != nil {
		t.Fatal(err)
	}
	muxer := &checkpointTestMuxer{}
	opts := options{
		sourceURL: server.URL + "/video.m3u8", title: "resume-test", outputDir: outputDir,
		concurrent: 1, jobID: "resume-test", userAgent: defaultUA,
	}
	if err := runWithOptions(opts, muxer); err != nil {
		t.Fatal(err)
	}
	if playlistRequests != 1 || segmentRequests != 1 || !muxer.called {
		t.Fatalf("playlist=%d segments=%d muxed=%v", playlistRequests, segmentRequests, muxer.called)
	}
}
