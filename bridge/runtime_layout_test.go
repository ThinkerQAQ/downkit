package downkit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestDataRootForExecutable(t *testing.T) {
	cases := []struct{ executable, root string }{
		{filepath.Join("C:", "software", "Coding", "downkit", "bin", "downkit.exe"),
			filepath.Join("C:", "software", "Coding", "downkit", "Data")},
		{filepath.Join("workspace", "downkit.exe"), filepath.Join("workspace", "Data")},
	}
	for _, c := range cases {
		if got := dataRootForExecutable(c.executable); got != c.root {
			t.Errorf("dataRootForExecutable(%q)=%q, want %q", c.executable, got, c.root)
		}
	}
}

func TestConfigTOMLGroupedRoundTrip(t *testing.T) {
	c := defaultBridgeConfig()
	c.OutputDir = "test-output"
	c.Concurrent = 32
	c.Quality = "best"
	c.UILanguage = "en"
	c.OpenMode = "sidePanel"
	c.ShowLogs = false
	c.DetectDash = false
	c.LogMaxMB = 20
	data, err := toml.Marshal(configToFile(c))
	if err != nil {
		t.Fatal(err)
	}
	var restored fileConfig
	if err := toml.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	want, err := normalizeBridgeConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := normalizeBridgeConfig(configFromFile(restored))
	if err != nil {
		t.Fatal(err)
	}
	if got.OutputDir != want.OutputDir || got.Concurrent != 32 || got.UILanguage != "en" ||
		got.OpenMode != "sidePanel" || got.ShowLogs || got.DetectDash || got.LogMaxMB != 20 {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestExistingTOMLDefaultsToShowingLogTab(t *testing.T) {
	var f fileConfig
	f.Interface.Language = "zh-CN"
	if !configFromFile(f).ShowLogs {
		t.Fatal("existing TOML without show_logs must default to true")
	}
	hidden := false
	f.Interface.ShowLogs = &hidden
	if configFromFile(f).ShowLogs {
		t.Fatal("explicitly hidden log tab must remain hidden")
	}
}

func TestRuntimeLogLineStructuredAndRedacted(t *testing.T) {
	line := runtimeLogLine("INFO", "bridge", "phase_changed", "jobId", "012345abcdef", "phase", "downloading", "progress", "25")
	for _, item := range []string{"level=INFO", "component=bridge", "event=phase_changed", "jobId=012345abcdef", "phase=downloading", "progress=25"} {
		if !strings.Contains(line, item) {
			t.Fatalf("missing %q: %s", item, line)
		}
	}
	for _, fields := range [][]string{
		{"url", "example.com"}, {"token", "abc"}, {"reason", "https://example.com/?cookie=secret"},
		{"jobId", "1234567890123456789012345678901234567890123456789012345678901234"},
		{"jobId", "not-a-job-id"}, {"progress", "101"}, {"phase", "bad value"},
	} {
		if got := runtimeLogLine("ERROR", "worker", "failed", fields...); got != "" {
			t.Fatalf("unsafe log attribute accepted: %q", got)
		}
	}
}
