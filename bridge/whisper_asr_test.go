package downkit

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWhisperASREngineProducesAndReusesSRT(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "中文课程.mp4")
	modelPath := filepath.Join(root, "ggml-small.bin")
	ffmpegPath := filepath.Join(root, "ffmpeg-test")
	whisperPath := filepath.Join(root, "whisper-cli-test")
	for path, contents := range map[string]string{
		mediaPath: "media", modelPath: "model", ffmpegPath: "tool", whisperPath: "tool",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	engine := &whisperASREngine{
		whisperPath: whisperPath, modelPath: modelPath, ffmpegPath: ffmpegPath,
		workDir: filepath.Join(root, "work"),
		run: func(_ context.Context, executable string, args []string, workDir string, _, _ io.Writer) error {
			calls++
			if executable == ffmpegPath {
				return os.WriteFile(args[len(args)-1], []byte("wav"), 0o600)
			}
			outputIndex := slices.Index(args, "-of")
			if workDir == "" || filepath.IsAbs(args[outputIndex+1]) {
				t.Fatalf("whisper output must use a relative ASCII staging path: dir=%q args=%#v", workDir, args)
			}
			return os.WriteFile(filepath.Join(workDir, args[outputIndex+1]+".srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nhello\n"), 0o644)
		},
	}
	artifact, err := engine.Transcribe(context.Background(), MediaOutput{Path: mediaPath, Index: 1, ItemID: "lesson"}, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || artifact.Source != "asr" || artifact.Format != "srt" || !strings.HasSuffix(artifact.Path, ".asr.srt") {
		t.Fatalf("unexpected ASR result: calls=%d artifact=%#v", calls, artifact)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Transcribe(context.Background(), MediaOutput{Path: mediaPath, Index: 1, ItemID: "lesson"}, "auto"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("completed ASR output was not reused; calls=%d", calls)
	}
}

func TestWhisperCommandArguments(t *testing.T) {
	audio := whisperAudioArgs("video.mp4", "audio.wav")
	if !slices.Contains(audio, "16000") || !slices.Contains(audio, "pcm_s16le") || !slices.Contains(audio, "wav") || audio[len(audio)-1] != "audio.wav" {
		t.Fatalf("unexpected FFmpeg args: %#v", audio)
	}
	whisper := whisperCLIArgs("model.bin", "audio.wav", "video.asr.srt", "auto")
	for _, required := range []string{"-osrt", "-of", "video.asr", "-l", "auto"} {
		if !slices.Contains(whisper, required) {
			t.Fatalf("missing %q in %#v", required, whisper)
		}
	}
	for _, required := range []string{"-progress", "pipe:1", "-nostats"} {
		if !slices.Contains(audio, required) {
			t.Fatalf("missing %q in %#v", required, audio)
		}
	}
	if !slices.Contains(whisper, "-pp") {
		t.Fatalf("Whisper native progress is disabled: %#v", whisper)
	}
}

func TestFFmpegAudioProgressReporterUsesMediaTimeline(t *testing.T) {
	type update struct {
		progress int
		detail   string
	}
	var updates []update
	reporter := newFFmpegAudioProgressReporter(func(progress int, detail string) {
		updates = append(updates, update{progress: progress, detail: detail})
	})
	reporter.logLine("Duration: 00:10:00.00, start: 0.000000, bitrate: 1000 kb/s")
	reporter.progressLine("out_time_us=150000000")
	reporter.progressLine("speed=20.0x")
	reporter.progressLine("progress=continue")
	reporter.progressLine("progress=end")
	if len(updates) < 3 || updates[len(updates)-2].progress != 25 || updates[len(updates)-1].progress != 100 {
		t.Fatalf("unexpected FFmpeg progress updates: %#v", updates)
	}
	if !strings.Contains(updates[len(updates)-2].detail, "02:30 / 10:00") {
		t.Fatalf("timeline detail missing: %#v", updates)
	}
}

func TestWhisperProgressReporterParsesNativeOutput(t *testing.T) {
	var progress []int
	reporter := newWhisperProgressReporter(func(value int, _ string) { progress = append(progress, value) })
	writer := newProgressLineWriter(reporter.line)
	_, _ = writer.Write([]byte("whisper_print_progress_callback: progress =  15%\r"))
	_, _ = writer.Write([]byte("whisper_print_progress_callback: progress =  45%\n"))
	_, _ = writer.Write([]byte("whisper_print_progress_callback: progress =  45%\n"))
	if !slices.Equal(progress, []int{15, 45}) {
		t.Fatalf("unexpected Whisper progress: %#v", progress)
	}
}

func TestCheckFFmpegASRCapabilities(t *testing.T) {
	if err := checkFFmpegASRCapabilities("Muxer wav", "Encoder pcm_s16le", "Filter aresample"); err != nil {
		t.Fatal(err)
	}
	err := checkFFmpegASRCapabilities("Unknown format 'wav'", "no encoders for it are available", "Unknown filter 'aresample'")
	if err == nil || !strings.Contains(err.Error(), "WAV muxer") || !strings.Contains(err.Error(), "pcm_s16le encoder") || !strings.Contains(err.Error(), "aresample filter") {
		t.Fatalf("unexpected capability error: %v", err)
	}
}
