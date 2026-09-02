package downkit

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNormalizeSubtitleRequest(t *testing.T) {
	request, err := normalizeSubtitleRequest(SubtitleRequest{
		Mode: " SITE ", Languages: []string{"zh.*", "zh.*", "en.*"}, IncludeAutomatic: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Mode != "site" || request.Format != "best" || len(request.Languages) != 2 || !request.IncludeAutomatic {
		t.Fatalf("unexpected normalized request: %#v", request)
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "translate"}); err == nil {
		t.Fatal("phase-one protocol accepted an unsupported mode")
	}
}

func TestReadYTDLPSubtitleArtifactsAssociatesMedia(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "video.mp4")
	subtitlePath := filepath.Join(root, "video.zh-Hans.vtt")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subtitlePath, []byte("WEBVTT"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "subtitles.txt")
	line := fmt.Sprintf("%s1|video-id|{\"zh-Hans\":{\"filepath\":%q,\"ext\":\"vtt\"}}\n", ytDLPSubtitleRecordPrefix, filepath.ToSlash(subtitlePath))
	if err := os.WriteFile(record, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	media := []MediaOutput{{Path: mediaPath, Index: 1, ItemID: "video-id"}}
	artifacts, err := readYTDLPSubtitleArtifacts(record, media, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Language != "zh-Hans" || artifacts[0].Format != "vtt" || artifacts[0].MediaPath != mediaPath {
		t.Fatalf("unexpected subtitle artifacts: %#v", artifacts)
	}
}

func TestYTDLPSubtitleOnlyArgsUseSourceSubtitleFlags(t *testing.T) {
	a := app{opts: options{}, workDir: t.TempDir()}
	args := a.ytDLPSubtitleOnlyArgs(
		"https://example.test/watch", MediaOutput{Path: filepath.Join(t.TempDir(), "video.mp4")}, "subtitles.txt",
		SubtitleRequest{Mode: "site", Languages: []string{"zh.*"}, IncludeAutomatic: true, Format: "best"}, ytDLPAttempt{},
	)
	for _, required := range []string{"--skip-download", "--write-subs", "--write-auto-subs", "--sub-langs", "zh.*"} {
		if !slices.Contains(args, required) {
			t.Fatalf("missing %q in %#v", required, args)
		}
	}
}
