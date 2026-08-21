package downkit

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseYTDLPProgress(t *testing.T) {
	progress, ok := parseYTDLPProgress("downkit-progress:5242880|10485760|NA|2621440.5| 50.0%")
	if !ok {
		t.Fatal("progress line was not recognized")
	}
	if progress.downloaded != 5242880 || progress.total != 10485760 || progress.speed != 2621440 {
		t.Fatalf("unexpected byte values: %+v", progress)
	}
	if progress.percent != 50 {
		t.Fatalf("progress = %d, want 50", progress.percent)
	}
}

func TestParseYTDLPProgressUsesEstimatedTotal(t *testing.T) {
	progress, ok := parseYTDLPProgress("downkit-progress:1024|NA|4096|512|25.0%")
	if !ok || progress.total != 4096 || progress.percent != 25 {
		t.Fatalf("unexpected estimated-total progress: %+v, ok=%v", progress, ok)
	}
}

func TestParseYTDLPProgressRejectsOrdinaryOutput(t *testing.T) {
	if _, ok := parseYTDLPProgress("[download] 50.0% of 10 MiB"); ok {
		t.Fatal("ordinary output was treated as progress")
	}
}

func TestParseYTDLPOutputIncludesPlaylistIdentity(t *testing.T) {
	output, ok := parseYTDLPOutput(`downkit-output:12|episode-id|C:\Downloads\Course\012 - lesson.mp4`)
	if !ok || output.index != 12 || output.id != "episode-id" || !strings.HasSuffix(output.path, "012 - lesson.mp4") {
		t.Fatalf("unexpected output: %#v, ok=%v", output, ok)
	}
}

func TestYTDLPDownloadArgsForceProgressWithPrint(t *testing.T) {
	a := app{opts: options{concurrent: 4}, workDir: t.TempDir()}
	args := a.ytDLPDownloadArgs("https://example.com/video", "video.%(ext)s", "outputs.txt", false, ytDLPAttempt{})
	if !slices.Contains(args, "--print") {
		t.Fatal("test setup requires --print, which makes yt-dlp quiet")
	}
	if !slices.Contains(args, "--progress") {
		t.Fatal("--progress is required to override --print's implicit quiet mode")
	}
	if !slices.Contains(args, "after_move:downkit-output:%(playlist_index)s|%(id)s|%(filepath)s") {
		t.Fatalf("missing per-file output report: %#v", args)
	}
	if !slices.Contains(args, "--continue") {
		t.Fatal("--continue is required for partial-file resume")
	}
	archiveIndex := slices.Index(args, "--download-archive")
	if archiveIndex < 0 || archiveIndex+1 >= len(args) || args[archiveIndex+1] != filepath.Join(a.workDir, "yt-dlp-archive.txt") {
		t.Fatalf("missing stable per-job archive: %#v", args)
	}
}

func TestDescribeYTDLPFailureExplainsBilibili412(t *testing.T) {
	err := describeYTDLPFailure(errors.New("exit status 1"), "ERROR: [BiliBili] test: HTTP Error 412: Precondition Failed")
	if err == nil || !strings.Contains(err.Error(), "B站") || !strings.Contains(err.Error(), "HTTP 412") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestYTDLPFallbacksUseExtractorIdentity(t *testing.T) {
	for _, extractor := range []string{"youtube", "youtube:tab"} {
		detail := "ERROR: [" + extractor + "] test: Requested format is not available"
		fallbacks := ytDLPFallbacks(detail)
		if len(fallbacks) != 1 || !slices.Contains(fallbacks[0].args, "youtube:player_client=web_safari") {
			t.Errorf("unexpected fallback for %q: %#v", extractor, fallbacks)
		}
	}
	if fallbacks := ytDLPFallbacks("ERROR: [vimeo] test: Requested format is not available"); len(fallbacks) != 0 {
		t.Fatalf("unsupported extractor received an unsafe fallback: %#v", fallbacks)
	}
	if fallbacks := ytDLPFallbacks("ERROR: [youtube] test: HTTP Error 403"); len(fallbacks) != 0 {
		t.Fatalf("unrelated error received a format fallback: %#v", fallbacks)
	}
}

func TestInsertYTDLPArgsBeforeURL(t *testing.T) {
	args := []string{"--format", "best", "https://www.youtube.com/watch?v=test"}
	got := insertYTDLPArgsBeforeURL(args, "--extractor-args", "youtube:player_client=web_safari")
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "youtube:player_client=web_safari") || got[len(got)-1] != args[len(args)-1] {
		t.Fatalf("unexpected fallback args: %#v", got)
	}
	if len(args) != 3 {
		t.Fatalf("input args were modified: %#v", args)
	}
}

func TestTailBufferKeepsRecentOutput(t *testing.T) {
	buffer := tailBuffer{limit: 8}
	_, _ = buffer.Write([]byte("123456"))
	_, _ = buffer.Write([]byte("7890"))
	if got := buffer.String(); got != "34567890" {
		t.Fatalf("tail = %q", got)
	}
}
