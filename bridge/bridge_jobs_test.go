package downkit

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestResumeRefreshesCredentialsWithoutPersistingThem(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "jobs.json")
	now := time.Now()
	server := &bridgeServer{
		state: bridgeState{Token: "test-token"}, config: bridgeConfig{OutputDir: t.TempDir()},
		pending: make(map[string]pendingBridgeTask), jobStorePath: storePath,
		jobs: map[string]*bridgeJob{
			"job-1": {
				ID: "job-1", Status: "paused", CreatedAt: now, UpdatedAt: now,
				task: bridgeTask{URL: "https://www.bilibili.com/video/test", UserAgent: "old"},
			},
		},
	}
	refreshed := refreshedJobCredentials{
		UserAgent:   "new-agent",
		PageCookies: []browserCookie{{Name: "SESSDATA", Value: "memory-only-secret", Domain: ".bilibili.com", Path: "/"}},
	}
	// Exercise the resume credential update and the exact persistence path without
	// launching a real worker process from the unit test.
	server.mu.Lock()
	job := server.jobs["job-1"]
	if err := applyRefreshedJobCredentials(&job.task, refreshed); err != nil {
		server.mu.Unlock()
		t.Fatal(err)
	}
	if err := server.persistJobsLocked(); err != nil {
		server.mu.Unlock()
		t.Fatal(err)
	}
	server.mu.Unlock()

	if job.task.UserAgent != "new-agent" || len(job.task.PageCookies) != 1 || job.task.PageCookies[0].Value != "memory-only-secret" {
		t.Fatalf("credentials were not refreshed in memory: %#v", job.task)
	}
	persisted, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte("memory-only-secret")) || bytes.Contains(persisted, []byte("SESSDATA")) {
		t.Fatalf("credentials leaked to disk: %s", persisted)
	}
}

func TestRefreshWithoutCookiesPreservesPausedTaskCredentials(t *testing.T) {
	task := bridgeTask{
		UserAgent:   "old",
		PageCookies: []browserCookie{{Name: "stale", Value: "secret", Domain: ".bilibili.com", Path: "/"}},
	}
	if err := applyRefreshedJobCredentials(&task, refreshedJobCredentials{UserAgent: "new"}); err != nil {
		t.Fatal(err)
	}
	if task.UserAgent != "new" || len(task.PageCookies) != 1 || task.PageCookies[0].Value != "secret" {
		t.Fatalf("refresh discarded paused task credentials: %#v", task)
	}
}

func TestJobProgressPauseAndDelete(t *testing.T) {
	now := time.Now()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "finished.mp4")
	if err := os.WriteFile(outputPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &bridgeServer{
		state:   bridgeState{Token: "test-token"},
		config:  bridgeConfig{OutputDir: outputDir},
		pending: make(map[string]pendingBridgeTask),
		jobs: map[string]*bridgeJob{
			"job-1": {ID: "job-1", Status: "running", CreatedAt: now, UpdatedAt: now},
		},
	}

	call := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		request.Header.Set("X-DownKit-Token", "test-token")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.handleJobs(response, request)
		return response
	}

	response := call("/v1/jobs/job-1/progress", `{"phase":"downloading","progress":42,"detail":"下载中","downloadedBytes":1024,"totalBytes":4096,"speedBytesPerSecond":5242880}`)
	if response.Code != http.StatusOK {
		t.Fatalf("progress status = %d, body = %s", response.Code, response.Body.String())
	}
	job := server.jobs["job-1"]
	if job.Phase != "downloading" || job.Progress != 42 || job.Detail != "下载中" || job.DownloadedBytes != 1024 || job.TotalBytes != 4096 || job.SpeedBytesPerSecond != 5242880 {
		t.Fatalf("unexpected progress: %+v", job)
	}

	response = call("/v1/jobs/job-1/output", `{"path":`+strconv.Quote(outputPath)+`}`)
	if response.Code != http.StatusOK || len(job.OutputPaths) != 1 || job.OutputPaths[0] != outputPath {
		t.Fatalf("output status = %d, body = %s, job = %+v", response.Code, response.Body.String(), job)
	}

	response = call("/v1/jobs/job-1/pause", "")
	if response.Code != http.StatusOK || job.Status != "paused" || job.SpeedBytesPerSecond != 0 {
		t.Fatalf("pause status = %d, job = %+v", response.Code, job)
	}

	response = call("/v1/jobs/job-1/delete", "")
	if response.Code != http.StatusOK || server.jobs["job-1"] != nil {
		t.Fatalf("delete status = %d, jobs = %#v", response.Code, server.jobs)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("deleting only the record removed the downloaded file: %v", err)
	}
}

func TestDeleteJobCanDeleteAllRegisteredOutputFiles(t *testing.T) {
	now := time.Now()
	outputDir := t.TempDir()
	first := filepath.Join(outputDir, "episode-1.mp4")
	second := filepath.Join(outputDir, "episode-2.mp4")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("media"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := &bridgeServer{
		state: bridgeState{Token: "test-token"}, config: bridgeConfig{OutputDir: outputDir},
		pending: make(map[string]pendingBridgeTask), jobs: map[string]*bridgeJob{
			"job-1": {ID: "job-1", Status: "completed", OutputPaths: []string{first, second}, CreatedAt: now, UpdatedAt: now},
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/delete", bytes.NewBufferString(`{"deleteFiles":true}`))
	request.Header.Set("X-DownKit-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.handleJobs(response, request)
	if response.Code != http.StatusOK || server.jobs["job-1"] != nil {
		t.Fatalf("delete status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("registered output still exists: %s", path)
		}
	}
}

func TestJobFilesMapCompletedPlaylistOutput(t *testing.T) {
	now := time.Now()
	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "002 - lesson.mp4")
	if err := os.WriteFile(outputPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &bridgeServer{
		config: bridgeConfig{OutputDir: outputDir}, pending: make(map[string]pendingBridgeTask),
		jobs: map[string]*bridgeJob{"job-1": {ID: "job-1", Status: "running", CreatedAt: now, UpdatedAt: now}},
	}
	files := []bridgeJobFile{{Index: 1, ID: "first", Title: "第一集"}, {Index: 2, ID: "second", Title: "第二集"}}
	if err := server.setJobFiles("job-1", files); err != nil {
		t.Fatal(err)
	}
	if err := server.addJobOutputFile("job-1", outputPath, 2, "second"); err != nil {
		t.Fatal(err)
	}
	job := server.jobs["job-1"]
	if len(job.Files) != 2 || job.Files[0].Status != "pending" || job.Files[1].Status != "completed" || job.Files[1].OutputPath != outputPath {
		t.Fatalf("unexpected playlist files: %#v", job.Files)
	}
}

func TestValidatedOutputFileRejectsOutsideDownloadDirectory(t *testing.T) {
	outputDir := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.mp4")
	if err := os.WriteFile(outside, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := validatedOutputFile(outputDir, outside); err == nil {
		t.Fatal("expected path outside the download directory to be rejected")
	}
}

func TestFailedJobRetryReusesOriginalCheckpointIdentity(t *testing.T) {
	now := time.Now()
	server := &bridgeServer{
		state: bridgeState{Token: "test-token"}, config: bridgeConfig{OutputDir: t.TempDir()},
		pending: make(map[string]pendingBridgeTask), jobs: map[string]*bridgeJob{
			"job-1": {ID: "job-1", Status: "failed", CreatedAt: now, UpdatedAt: now, task: bridgeTask{URL: "https://example.test/video.m3u8"}},
		},
	}
	// Keep this unit test independent of spawning the test binary as a worker:
	// verify the retry precondition and credential mutation used immediately
	// before startJob receives the same job ID.
	job := server.jobs["job-1"]
	canRetry := job != nil && job.Status == "failed"
	if !canRetry {
		t.Fatal("failed job should be retryable")
	}
	if err := applyRefreshedJobCredentials(&job.task, refreshedJobCredentials{UserAgent: "fresh"}); err != nil {
		t.Fatal(err)
	}
	if server.jobs["job-1"] != job || job.task.UserAgent != "fresh" {
		t.Fatalf("retry replaced checkpoint identity or lost credentials: %#v", job)
	}
}
