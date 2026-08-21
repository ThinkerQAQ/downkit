package downkit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type muxRequest struct {
	WorkDir string
	Video   string
	Audio   string
	Output  string
}

type mediaMuxer interface {
	Mux(muxRequest) error
}

type ffmpegMuxer struct {
	path   string
	stdout io.Writer
	stderr io.Writer
}

func (m ffmpegMuxer) Mux(request muxRequest) error {
	partial := strings.TrimSuffix(request.Output, filepath.Ext(request.Output)) + ".partial.mp4"
	args := ffmpegMuxArgs(request, partial)

	cmd := exec.Command(m.path, args...)
	cmd.Dir = request.WorkDir
	cmd.Stdout, cmd.Stderr = m.stdout, m.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg 失败：%w", err)
	}
	return os.Rename(partial, request.Output)
}

func ffmpegMuxArgs(request muxRequest, partial string) []string {
	video := request.Video
	if video == "" {
		video = "local.m3u8"
	}
	args := []string{
		"-hide_banner", "-y", "-loglevel", "warning",
		"-allowed_extensions", "ALL", "-protocol_whitelist", "file,crypto,data",
		"-i", video,
	}
	if request.Audio != "" {
		args = append(args, "-i", request.Audio, "-map", "0:v?", "-map", "1:a?", "-c", "copy")
	} else {
		args = append(args, "-map", "0:v?", "-map", "0:a?", "-c", "copy")
	}
	args = append(args, "-movflags", "+faststart", partial)
	return args
}

func (a *app) mux(output, audioInput string) error {
	if a.muxer == nil {
		return fmt.Errorf("没有可用的媒体封装器")
	}
	fmt.Fprintln(consoleOut, "无损封装 MP4")
	return a.muxer.Mux(muxRequest{
		WorkDir: a.workDir,
		Video:   "local.m3u8",
		Audio:   audioInput,
		Output:  output,
	})
}
