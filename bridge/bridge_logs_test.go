package downkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLogsReadFilterAndClear(t *testing.T) {
	directory := t.TempDir()
	bridge := "2026-10-11T10:00:00+08:00 component=bridge event=ready\n2026-10-11T10:02:00+08:00 component=bridge event=stopped\n"
	worker := "2026-10-11T10:01:00+08:00 component=worker event=failed\n"
	if err := os.WriteFile(filepath.Join(directory, "bridge.log"), []byte(bridge), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "worker.log"), []byte(worker), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := readRuntimeLogs(directory, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Level != "INFO" || entries[1].Level != "ERROR" {
		t.Fatalf("unexpected chronological log entries: %#v", entries)
	}
	if !strings.Contains(entries[1].Raw, "event=failed") {
		t.Fatalf("missing worker event: %#v", entries[1])
	}
	entries, err = readRuntimeLogs(directory, 2)
	if err != nil || len(entries) != 2 || entries[0].Level != "ERROR" {
		t.Fatalf("tail limit failed: %#v, %v", entries, err)
	}
	if err := clearRuntimeLogs(directory); err != nil {
		t.Fatal(err)
	}
	entries, err = readRuntimeLogs(directory, 100)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cleared logs still visible: %#v %v", entries, err)
	}
}

func TestRuntimeLogsBoundedInputAndMissingFiles(t *testing.T) {
	directory := t.TempDir()
	entries, err := readRuntimeLogs(directory, maxLogEntries)
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty log directory: %#v %v", entries, err)
	}
	// After file-tail seek the first partial record must be discarded.
	prefix := strings.Repeat("A", maxLogFileTail+50)
	line := "2026-10-11T10:03:00+08:00 component=bridge event=ready\n"
	if err := os.WriteFile(filepath.Join(directory, "bridge.log"), []byte(prefix+"\n"+line), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err = readRuntimeLogs(directory, maxLogEntries+100)
	if err != nil || len(entries) != 1 || entries[0].Message != "component=bridge event=ready" {
		t.Fatalf("unexpected bounded tail entries: %#v %v", entries, err)
	}
}

func TestRuntimeLogsRequireToken(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: strings.Repeat("x", 64)}}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		request := httptest.NewRequest(method, "/v1/logs", nil)
		recorder := httptest.NewRecorder()
		server.handleLogs(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s accepted request without token: %d", method, recorder.Code)
		}
	}
}

func TestRuntimeLogsStructuredSeverityWithoutDuplicateLevel(t *testing.T) {
	dir := t.TempDir()
	line := "2026-10-11T10:00:00+08:00 level=WARN component=bridge event=restart_rejected reason=active_jobs\n"
	if err := os.WriteFile(filepath.Join(dir, "bridge.log"), []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := readRuntimeLogs(dir, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read: %#v %v", entries, err)
	}
	if entries[0].Level != "WARN" || strings.Count(entries[0].Raw, "WARN") != 1 {
		t.Fatalf("structured level is duplicated: %#v", entries[0])
	}
}
