package downkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type recordingASREngine struct {
	called bool
}

type recordingTranslator struct {
	called bool
	closed bool
}

func (t *recordingTranslator) Translate(_ context.Context, artifact SubtitleArtifact, target string, bilingual bool) (SubtitleArtifact, error) {
	t.called = true
	artifact.Path += ".translated.srt"
	artifact.Language = target
	artifact.Source = "translation"
	return artifact, nil
}

func (t *recordingTranslator) Close() error {
	t.closed = true
	return nil
}

func (e *recordingASREngine) Transcribe(_ context.Context, media MediaOutput, language string) (SubtitleArtifact, error) {
	e.called = true
	return SubtitleArtifact{Path: media.Path + ".srt", MediaPath: media.Path, Language: language, Format: "srt", Source: "asr"}, nil
}

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
	fallback, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "site-or-asr"})
	if err != nil || fallback.ASRLanguage != "auto" || len(fallback.Languages) != 2 || !fallback.needsASR() || !fallback.usesSiteSubtitles() {
		t.Fatalf("unexpected fallback request: %#v, %v", fallback, err)
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "asr", ASRLanguage: "bad language"}); err == nil {
		t.Fatal("invalid ASR language was accepted")
	}
	translated, err := normalizeSubtitleRequest(SubtitleRequest{
		Mode: "asr", ASRLanguage: "ja", TargetLanguage: " ZH-HANS ", Bilingual: true,
	})
	if err != nil || translated.TargetLanguage != "zh-Hans" || !translated.Bilingual || !translated.needsTranslation() {
		t.Fatalf("unexpected translation request: %#v, %v", translated, err)
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "asr", TargetLanguage: "bad language"}); err == nil {
		t.Fatal("invalid target language was accepted")
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "asr", Bilingual: true}); err == nil {
		t.Fatal("bilingual request without a target language was accepted")
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "none", TargetLanguage: "zh-Hans"}); err == nil {
		t.Fatal("translation without a subtitle source was accepted")
	}
	if _, err := normalizeSubtitleRequest(SubtitleRequest{Mode: "site-or-asr", ASRLanguage: "auto", TargetLanguage: "zh-Hans"}); err == nil {
		t.Fatal("translation with automatic fallback ASR language was accepted")
	}
}

func TestSubtitlePipelineRejectsTranslationWithoutTranslator(t *testing.T) {
	pipeline := newSubtitlePipeline(&app{}, &recordingASREngine{}, nil)
	_, err := pipeline.Process(context.Background(), SubtitleRequest{
		Mode: "asr", ASRLanguage: "ja", TargetLanguage: "zh-Hans",
	}, []MediaOutput{{Path: "video.mp4"}})
	if err == nil || !strings.Contains(err.Error(), "翻译引擎尚未配置") {
		t.Fatalf("translation without an engine should fail clearly: %v", err)
	}
}

func TestSubtitlePipelineFallsBackToASR(t *testing.T) {
	engine := &recordingASREngine{}
	pipeline := newSubtitlePipeline(&app{}, engine, nil)
	media := []MediaOutput{{Path: "video.mp4", SubtitleAttempted: true}}
	artifacts, err := pipeline.Process(context.Background(), SubtitleRequest{Mode: "site-or-asr", ASRLanguage: "auto"}, media)
	if err != nil || !engine.called || len(artifacts) != 1 || artifacts[0].Source != "asr" {
		t.Fatalf("ASR fallback failed: artifacts=%#v called=%v err=%v", artifacts, engine.called, err)
	}
}

func TestSubtitlePipelinePrefersExistingSiteArtifact(t *testing.T) {
	engine := &recordingASREngine{}
	pipeline := newSubtitlePipeline(&app{}, engine, nil)
	media := []MediaOutput{{Path: "video.mp4", SubtitleArtifacts: []SubtitleArtifact{{Path: "video.zh.vtt", Source: "site"}}}}
	artifacts, err := pipeline.Process(context.Background(), SubtitleRequest{Mode: "site-or-asr", ASRLanguage: "auto"}, media)
	if err != nil || engine.called || len(artifacts) != 1 || artifacts[0].Source != "site" {
		t.Fatalf("site subtitle was not preferred: artifacts=%#v called=%v err=%v", artifacts, engine.called, err)
	}
}

func TestSubtitlePipelineTranslatesAcquiredArtifact(t *testing.T) {
	engine := &recordingASREngine{}
	translator := &recordingTranslator{}
	pipeline := newSubtitlePipeline(&app{}, engine, translator)
	media := []MediaOutput{{Path: "video.mp4", SubtitleAttempted: true}}
	artifacts, err := pipeline.Process(context.Background(), SubtitleRequest{
		Mode: "site-or-asr", ASRLanguage: "ja", TargetLanguage: "zh-Hans", Bilingual: true,
	}, media)
	if err != nil || !engine.called || !translator.called || !translator.closed || len(artifacts) != 2 {
		t.Fatalf("translation pipeline failed: artifacts=%#v engine=%v translator=%#v err=%v", artifacts, engine.called, translator, err)
	}
	if artifacts[0].Source != "asr" || artifacts[1].Source != "translation" || artifacts[1].Language != "zh-Hans" {
		t.Fatalf("unexpected translated artifacts: %#v", artifacts)
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
