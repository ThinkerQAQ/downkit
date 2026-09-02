package downkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fixedSubtitlePipeline struct {
	called    bool
	artifacts []SubtitleArtifact
}

func (p *fixedSubtitlePipeline) Process(_ context.Context, _ SubtitleRequest, _ []MediaOutput) ([]SubtitleArtifact, error) {
	p.called = true
	return p.artifacts, nil
}

func TestFinalizeOutputsOwnsCleanup(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "video.mp4")
	workDir := filepath.Join(root, ".downkit-work", "job-test")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{opts: options{}, workDir: workDir}
	if err := a.finalizeOutputs([]MediaOutput{{Path: mediaPath, Title: "video"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("work directory was not removed: %v", err)
	}
}

func TestFinalizeOutputsRunsConfiguredSubtitlePipeline(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "video.mp4")
	subtitlePath := filepath.Join(root, "video.zh.srt")
	for path, contents := range map[string]string{mediaPath: "media", subtitlePath: "subtitle"} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pipeline := &fixedSubtitlePipeline{artifacts: []SubtitleArtifact{{Path: subtitlePath, Language: "zh", Source: "site"}}}
	a := &app{
		opts:    options{keepWork: true, subtitleRequest: SubtitleRequest{Mode: "site"}},
		workDir: root, subtitlePipeline: pipeline,
	}
	if err := a.finalizeOutputs([]MediaOutput{{Path: mediaPath, Title: "video"}}); err != nil {
		t.Fatal(err)
	}
	if !pipeline.called {
		t.Fatal("subtitle pipeline was not called")
	}
}
