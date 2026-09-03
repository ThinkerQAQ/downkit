package downkit

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type asrCommandRunner func(context.Context, string, []string, string, io.Writer, io.Writer) error

type whisperASREngine struct {
	whisperPath string
	modelPath   string
	ffmpegPath  string
	workDir     string
	keepWork    bool
	run         asrCommandRunner
}

func newWhisperASREngine(whisperPath, modelPath, ffmpegPath, workDir string, keepWork bool) ASREngine {
	return &whisperASREngine{
		whisperPath: whisperPath, modelPath: modelPath, ffmpegPath: ffmpegPath,
		workDir: workDir, keepWork: keepWork, run: runASRCommand,
	}
}

func runASRCommand(ctx context.Context, executable string, args []string, workDir string, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workDir
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func validateWhisperModel(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("本地语音识别尚未配置 whisper.cpp 模型路径")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("找不到 whisper.cpp 模型：%s", path)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("whisper.cpp 模型文件无效：%s", path)
	}
	return nil
}

func checkFFmpegASRCapabilities(muxerHelp, encoderHelp, filterHelp string) error {
	missing := make([]string, 0, 3)
	if !strings.Contains(strings.ToLower(muxerHelp), "muxer wav") {
		missing = append(missing, "WAV muxer")
	}
	if !strings.Contains(strings.ToLower(encoderHelp), "encoder pcm_s16le") {
		missing = append(missing, "pcm_s16le encoder")
	}
	if !strings.Contains(strings.ToLower(filterHelp), "filter aresample") {
		missing = append(missing, "aresample filter")
	}
	if len(missing) > 0 {
		return fmt.Errorf("FFmpeg 缺少本地语音识别能力：%s", strings.Join(missing, "、"))
	}
	return nil
}

func validateFFmpegASRSupport(path string) error {
	queries := [][]string{
		{"-hide_banner", "-h", "muxer=wav"},
		{"-hide_banner", "-h", "encoder=pcm_s16le"},
		{"-hide_banner", "-h", "filter=aresample"},
	}
	outputs := make([]string, len(queries))
	for index, args := range queries {
		output, err := exec.Command(path, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("无法检查 FFmpeg 本地语音识别能力：%w", err)
		}
		outputs[index] = string(output)
	}
	if err := checkFFmpegASRCapabilities(outputs[0], outputs[1], outputs[2]); err != nil {
		return fmt.Errorf("%w；请使用包含 WAV/PCM/重采样支持的 DownKit FFmpeg Slim", err)
	}
	fmt.Fprintln(consoleOut, "本地 ASR：FFmpeg WAV/PCM/重采样能力就绪")
	return nil
}

func (e *whisperASREngine) Transcribe(ctx context.Context, media MediaOutput, language string) (SubtitleArtifact, error) {
	if err := validateWhisperModel(e.modelPath); err != nil {
		return SubtitleArtifact{}, err
	}
	whisperPath, err := findTool(e.whisperPath, "whisper-cli")
	if err != nil {
		return SubtitleArtifact{}, err
	}
	ffmpegPath, err := findTool(e.ffmpegPath, "ffmpeg")
	if err != nil {
		return SubtitleArtifact{}, err
	}
	if language == "" {
		language = "auto"
	}
	asrDir := filepath.Join(e.workDir, "asr")
	if err := os.MkdirAll(asrDir, 0o755); err != nil {
		return SubtitleArtifact{}, err
	}
	key := whisperMediaKey(media)
	audioPath := filepath.Join(asrDir, key+".wav")
	outputPath, err := resumableASROutputPath(asrDir, key, media.Path)
	if err != nil {
		return SubtitleArtifact{}, err
	}
	artifact := SubtitleArtifact{
		Path: outputPath, Language: language, Format: "srt", Source: "asr",
		MediaPath: media.Path, MediaIndex: media.Index, MediaID: media.ItemID,
	}
	if info, statErr := os.Stat(outputPath); statErr == nil && info.Mode().IsRegular() && info.Size() > 0 {
		publishJobPhaseProgress("transcribing", 100, "已复用完成的本地识别字幕", 0, 0, 0)
		fmt.Fprintln(consoleOut, "复用已完成的本地识别字幕：", outputPath)
		return artifact, nil
	}

	publishJobPhaseProgress("extracting-audio", 0, "正在读取媒体时长并提取 16 kHz 音频", 0, 0, 0)
	extractStarted := time.Now()
	fmt.Fprintf(consoleOut, "timestamp=%s level=INFO node=whisper-asr operation=extract-audio status=start media=%q\n", time.Now().Format(time.RFC3339), filepath.Base(media.Path))
	var ffmpegError tailBuffer
	ffmpegError.limit = 32 * 1024
	ffmpegProgress := newFFmpegAudioProgressReporter(func(progress int, detail string) {
		publishJobPhaseProgress("extracting-audio", progress, detail, 0, 0, 0)
	})
	ffmpegStdout := newProgressLineWriter(ffmpegProgress.progressLine)
	ffmpegStderr := newProgressLineWriter(ffmpegProgress.logLine)
	if err := e.run(ctx, ffmpegPath, whisperAudioArgs(media.Path, audioPath), "", ffmpegStdout, io.MultiWriter(&ffmpegError, ffmpegStderr)); err != nil {
		fmt.Fprintf(consoleErr, "timestamp=%s level=ERROR node=whisper-asr operation=extract-audio status=failed durationMs=%d error=%q\n", time.Now().Format(time.RFC3339), time.Since(extractStarted).Milliseconds(), err)
		return SubtitleArtifact{}, commandFailure("FFmpeg 音频提取失败", err, ffmpegError.String())
	}
	publishJobPhaseProgress("extracting-audio", 100, "音频提取完成", 0, 0, 0)
	fmt.Fprintf(consoleOut, "timestamp=%s level=INFO node=whisper-asr operation=extract-audio status=completed durationMs=%d\n", time.Now().Format(time.RFC3339), time.Since(extractStarted).Milliseconds())

	publishJobPhaseProgress("transcribing", 0, "正在加载 Whisper 模型并准备识别", 0, 0, 0)
	transcribeStarted := time.Now()
	fmt.Fprintf(consoleOut, "timestamp=%s level=INFO node=whisper-asr operation=transcribe status=start language=%s\n", time.Now().Format(time.RFC3339), language)
	var whisperError tailBuffer
	whisperError.limit = 64 * 1024
	whisperProgress := newWhisperProgressReporter(func(progress int, detail string) {
		publishJobPhaseProgress("transcribing", progress, detail, 0, 0, 0)
	})
	stderr := io.MultiWriter(consoleErr, &whisperError, newProgressLineWriter(whisperProgress.line))
	stagedOutputPath := filepath.Join(asrDir, key+".transcript.srt")
	_ = os.Remove(stagedOutputPath)
	if err := e.run(ctx, whisperPath, whisperCLIArgs(e.modelPath, filepath.Base(audioPath), filepath.Base(stagedOutputPath), language), asrDir, consoleOut, stderr); err != nil {
		fmt.Fprintf(consoleErr, "timestamp=%s level=ERROR node=whisper-asr operation=transcribe status=failed durationMs=%d error=%q\n", time.Now().Format(time.RFC3339), time.Since(transcribeStarted).Milliseconds(), err)
		return SubtitleArtifact{}, commandFailure("whisper.cpp 识别失败", err, whisperError.String())
	}
	publishJobPhaseProgress("transcribing", 100, "语音识别完成", 0, 0, 0)
	info, err := os.Stat(stagedOutputPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return SubtitleArtifact{}, errors.New("whisper.cpp 已退出但没有生成有效的 SRT 文件")
	}
	if err := os.Rename(stagedOutputPath, outputPath); err != nil {
		return SubtitleArtifact{}, fmt.Errorf("无法保存本地识别字幕：%w", err)
	}
	fmt.Fprintf(consoleOut, "timestamp=%s level=INFO node=whisper-asr operation=transcribe status=completed durationMs=%d output=%q\n", time.Now().Format(time.RFC3339), time.Since(transcribeStarted).Milliseconds(), outputPath)
	if !e.keepWork {
		_ = os.Remove(audioPath)
	}
	return artifact, nil
}

func whisperAudioArgs(mediaPath, audioPath string) []string {
	return []string{
		"-hide_banner", "-loglevel", "info", "-nostats", "-progress", "pipe:1", "-y", "-i", mediaPath,
		"-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "-f", "wav", audioPath,
	}
}

func whisperCLIArgs(modelPath, audioPath, outputPath, language string) []string {
	outputBase := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	return []string{
		"-m", modelPath, "-f", audioPath, "-l", language,
		"-osrt", "-of", outputBase, "-np", "-pp",
	}
}

func whisperMediaKey(media MediaOutput) string {
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(media.Path)) + "\n" + media.ItemID + fmt.Sprintf("\n%d", media.Index)))
	return fmt.Sprintf("%x", digest[:8])
}

func resumableASROutputPath(asrDir, key, mediaPath string) (string, error) {
	statePath := filepath.Join(asrDir, key+".output.txt")
	if data, err := os.ReadFile(statePath); err == nil {
		if saved := strings.TrimSpace(string(data)); saved != "" {
			return saved, nil
		}
	}
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath)) + ".asr.srt"
	outputPath := uniqueOutput(filepath.Dir(mediaPath), base)
	if err := os.WriteFile(statePath, []byte(outputPath), 0o600); err != nil {
		return "", err
	}
	return outputPath, nil
}

func commandFailure(label string, err error, detail string) error {
	detail = strings.TrimSpace(detail)
	if len(detail) > 800 {
		detail = detail[len(detail)-800:]
	}
	if detail == "" {
		return fmt.Errorf("%s：%w", label, err)
	}
	return fmt.Errorf("%s：%w：%s", label, err, detail)
}
