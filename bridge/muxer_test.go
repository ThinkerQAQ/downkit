package downkit

import (
	"reflect"
	"testing"
)

func TestFFmpegMuxerCommandShape(t *testing.T) {
	request := muxRequest{WorkDir: "work", Video: "local.m3u8", Audio: "audio/local.m3u8", Output: "out.mp4"}
	args := ffmpegMuxArgs(request, "out.partial.mp4")
	want := []string{
		"-hide_banner", "-y", "-loglevel", "warning",
		"-allowed_extensions", "ALL", "-protocol_whitelist", "file,crypto,data",
		"-i", "local.m3u8", "-i", "audio/local.m3u8",
		"-map", "0:v?", "-map", "1:a?", "-c", "copy",
		"-movflags", "+faststart", "out.partial.mp4",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args mismatch\n got: %#v\nwant: %#v", args, want)
	}
}
