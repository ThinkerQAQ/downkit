package downkit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	llamaModelLoadTimeout = 5 * time.Minute
	llamaRequestTimeout   = 3 * time.Minute
)

type llamaTranslator struct {
	serverPath  string
	modelPath   string
	ffmpegPath  string
	workDir     string
	baseURL     string
	client      *http.Client
	process     *exec.Cmd
	processDone chan error
	serverLog   tailBuffer
}

type llamaChatRequest struct {
	Model       string             `json:"model"`
	Messages    []llamaChatMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
	MaxTokens   int                `json:"max_tokens"`
}

type llamaChatMessage struct {
	Role    string             `json:"role"`
	Content []translateContent `json:"content"`
}

type translateContent struct {
	Type           string `json:"type"`
	SourceLanguage string `json:"source_lang_code"`
	TargetLanguage string `json:"target_lang_code"`
	Text           string `json:"text"`
}

type llamaChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func newLlamaTranslator(serverPath, modelPath, ffmpegPath, workDir string) Translator {
	return &llamaTranslator{
		serverPath: serverPath, modelPath: modelPath, ffmpegPath: ffmpegPath, workDir: workDir,
		client: &http.Client{Timeout: llamaRequestTimeout},
	}
}

func validateTranslationModel(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("本地字幕翻译尚未配置 GGUF 模型路径")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("找不到字幕翻译模型：%s", path)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 4 {
		return fmt.Errorf("字幕翻译模型文件无效：%s", path)
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(file, magic); err != nil || string(magic) != "GGUF" {
		return fmt.Errorf("字幕翻译模型不是有效的 GGUF 文件：%s", path)
	}
	return nil
}

func (t *llamaTranslator) Translate(ctx context.Context, artifact SubtitleArtifact, targetLanguage string, bilingual bool) (SubtitleArtifact, error) {
	started := time.Now()
	sourceLanguage, err := translationSourceLanguage(artifact.Language)
	if err != nil {
		return SubtitleArtifact{}, err
	}
	targetLanguage = normalizeSubtitleLanguage(targetLanguage)
	if !validSubtitleLanguage(targetLanguage) {
		return SubtitleArtifact{}, fmt.Errorf("字幕输出语言无效：%s", targetLanguage)
	}

	sourcePath, err := t.ensureSRT(ctx, artifact)
	if err != nil {
		return SubtitleArtifact{}, err
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return SubtitleArtifact{}, fmt.Errorf("无法读取待翻译字幕：%w", err)
	}
	document, err := parseSRT(data)
	if err != nil {
		return SubtitleArtifact{}, fmt.Errorf("无法解析待翻译字幕：%w", err)
	}
	translationDir := filepath.Join(t.workDir, "translation", translationArtifactKey(artifact, targetLanguage, bilingual))
	if err := os.MkdirAll(translationDir, 0o755); err != nil {
		return SubtitleArtifact{}, err
	}
	outputPath, err := resumableTranslationOutputPath(translationDir, artifact, targetLanguage, bilingual)
	if err != nil {
		return SubtitleArtifact{}, err
	}
	result := SubtitleArtifact{
		Path: outputPath, Language: targetLanguage, Format: "srt", Source: "translation",
		MediaPath: artifact.MediaPath, MediaIndex: artifact.MediaIndex, MediaID: artifact.MediaID,
	}
	if existing, readErr := os.ReadFile(outputPath); readErr == nil {
		if _, parseErr := parseSRT(existing); parseErr == nil {
			fmt.Fprintln(consoleOut, "本地翻译：复用已完成字幕：", outputPath)
			return result, nil
		}
	}

	checkpointPath := filepath.Join(translationDir, "translated.json")
	translations := loadTranslationCheckpoint(checkpointPath, document)
	for index, cue := range document.Cues {
		if strings.TrimSpace(translations[cue.ID]) != "" {
			continue
		}
		if err := t.ensureServer(ctx); err != nil {
			return SubtitleArtifact{}, err
		}
		publishJobPhaseProgress("processing", 86+index*10/max(len(document.Cues), 1), fmt.Sprintf("正在翻译字幕（%d/%d）", index+1, len(document.Cues)), 0, 0, 0)
		translated, translateErr := t.translateCueWithRetry(ctx, sourceLanguage, targetLanguage, cue.Text)
		if translateErr != nil {
			fmt.Fprintf(consoleErr, "本地翻译：失败 cue=%d durationMs=%d error=%v\n", cue.ID, time.Since(started).Milliseconds(), translateErr)
			return SubtitleArtifact{}, fmt.Errorf("字幕 %d 翻译失败：%w", cue.ID, translateErr)
		}
		translations[cue.ID] = translated
		if err := saveTranslationCheckpoint(checkpointPath, translations); err != nil {
			return SubtitleArtifact{}, err
		}
	}
	translatedDocument, err := document.withTranslations(translations, bilingual)
	if err != nil {
		return SubtitleArtifact{}, err
	}
	encoded, err := translatedDocument.marshalSRT()
	if err != nil {
		return SubtitleArtifact{}, err
	}
	if err := writeTranslationFile(outputPath, encoded); err != nil {
		return SubtitleArtifact{}, fmt.Errorf("无法保存翻译字幕：%w", err)
	}
	fmt.Fprintf(consoleOut, "本地翻译：完成 cues=%d durationMs=%d output=%s\n", len(document.Cues), time.Since(started).Milliseconds(), outputPath)
	return result, nil
}

func (t *llamaTranslator) Close() error {
	if t.process == nil || t.process.Process == nil {
		return nil
	}
	if t.process.ProcessState != nil && t.process.ProcessState.Exited() {
		t.process = nil
		return nil
	}
	fmt.Fprintln(consoleOut, "本地翻译：正在停止 llama-server")
	stopErr := stopProcess(t.process)
	if t.processDone != nil {
		select {
		case <-t.processDone:
		case <-time.After(5 * time.Second):
			return errors.New("llama-server 未在超时时间内退出")
		}
	}
	t.process = nil
	return stopErr
}

func (t *llamaTranslator) ensureServer(ctx context.Context) error {
	if t.baseURL != "" {
		return nil
	}
	if err := validateTranslationModel(t.modelPath); err != nil {
		return err
	}
	serverPath, err := findTool(t.serverPath, "llama-server")
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("无法为 llama-server 分配本机端口：%w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	t.serverLog.limit = 64 * 1024
	command := exec.Command(serverPath,
		"--model", t.modelPath, "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"--ctx-size", "2048", "--parallel", "1", "--n-gpu-layers", "0", "--no-webui",
	)
	configureSidecarCommand(command)
	command.Stdout = &t.serverLog
	command.Stderr = &t.serverLog
	fmt.Fprintf(consoleOut, "本地翻译：启动 llama-server port=%d model=%s\n", port, filepath.Base(t.modelPath))
	if err := command.Start(); err != nil {
		return fmt.Errorf("无法启动 llama-server：%w", err)
	}
	t.process = command
	t.processDone = make(chan error, 1)
	go func() { t.processDone <- command.Wait() }()
	t.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForLlamaHealth(ctx, t.client, t.baseURL, t.processDone); err != nil {
		_ = t.Close()
		detail := strings.TrimSpace(t.serverLog.String())
		return commandFailure("llama-server 启动失败", err, detail)
	}
	fmt.Fprintln(consoleOut, "本地翻译：llama-server 已就绪")
	return nil
}

func waitForLlamaHealth(ctx context.Context, client *http.Client, baseURL string, processDone <-chan error) error {
	deadline := time.NewTimer(llamaModelLoadTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-processDone:
			if err == nil {
				err = errors.New("进程已退出")
			}
			return err
		case <-deadline.C:
			return errors.New("等待模型加载超时")
		case <-ticker.C:
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
			if err != nil {
				return err
			}
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4*1024))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
	}
}

func (t *llamaTranslator) translateCueWithRetry(ctx context.Context, sourceLanguage, targetLanguage, text string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		translated, err := t.translateCue(ctx, sourceLanguage, targetLanguage, text)
		if err == nil {
			return translated, nil
		}
		lastErr = err
		fmt.Fprintf(consoleErr, "本地翻译：请求重试 attempt=%d result=error error=%v\n", attempt, err)
	}
	return "", lastErr
}

func (t *llamaTranslator) translateCue(ctx context.Context, sourceLanguage, targetLanguage, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("原字幕文本为空")
	}
	payload := llamaChatRequest{
		Model: filepath.Base(t.modelPath), Temperature: 0,
		MaxTokens: min(1024, max(64, len([]rune(text))*4+32)),
		Messages: []llamaChatMessage{{Role: "user", Content: []translateContent{{
			Type: "text", SourceLanguage: sourceLanguage, TargetLanguage: targetLanguage, Text: text,
		}}}},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := t.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	var result llamaChatResponse
	if json.Unmarshal(data, &result) != nil {
		return "", fmt.Errorf("llama-server 返回了无效 JSON（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(result.Error.Message)
		if detail == "" {
			detail = http.StatusText(response.StatusCode)
		}
		return "", fmt.Errorf("llama-server HTTP %d：%s", response.StatusCode, detail)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("llama-server 没有返回翻译文本")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func translationSourceLanguage(language string) (string, error) {
	language = normalizeSubtitleLanguage(strings.TrimSuffix(strings.TrimSpace(language), ".*"))
	if language == "" || language == "auto" || language == "all" {
		return "", errors.New("翻译前必须明确选择视频语音；当前字幕没有可靠的原语言标记")
	}
	if strings.HasSuffix(strings.ToLower(language), "-orig") {
		language = language[:len(language)-len("-orig")]
	}
	if !validSubtitleLanguage(language) {
		return "", fmt.Errorf("字幕原语言标记无效：%s", language)
	}
	return language, nil
}

func (t *llamaTranslator) ensureSRT(ctx context.Context, artifact SubtitleArtifact) (string, error) {
	if strings.EqualFold(filepath.Ext(artifact.Path), ".srt") || strings.EqualFold(artifact.Format, "srt") {
		return artifact.Path, nil
	}
	ffmpegPath, err := findTool(t.ffmpegPath, "ffmpeg")
	if err != nil {
		return "", err
	}
	directory := filepath.Join(t.workDir, "translation", "sources")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(artifact.Path))))
	output := filepath.Join(directory, fmt.Sprintf("%x.srt", digest[:8]))
	if data, readErr := os.ReadFile(output); readErr == nil {
		if _, parseErr := parseSRT(data); parseErr == nil {
			return output, nil
		}
	}
	var detail tailBuffer
	detail.limit = 32 * 1024
	command := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-i", artifact.Path, "-f", "srt", output)
	command.Stdout = io.Discard
	command.Stderr = &detail
	if err := command.Run(); err != nil {
		return "", commandFailure("FFmpeg 字幕转换失败", err, detail.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return "", err
	}
	if _, err := parseSRT(data); err != nil {
		return "", fmt.Errorf("FFmpeg 生成的 SRT 无效：%w", err)
	}
	return output, nil
}

func translationArtifactKey(artifact SubtitleArtifact, targetLanguage string, bilingual bool) string {
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(artifact.Path)) + "\n" + targetLanguage + "\n" + strconv.FormatBool(bilingual)))
	return fmt.Sprintf("%x", digest[:8])
}

func resumableTranslationOutputPath(directory string, artifact SubtitleArtifact, targetLanguage string, bilingual bool) (string, error) {
	statePath := filepath.Join(directory, "output.txt")
	if data, err := os.ReadFile(statePath); err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data)), nil
	}
	mediaPath := artifact.MediaPath
	if strings.TrimSpace(mediaPath) == "" {
		mediaPath = artifact.Path
	}
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath)) + "." + targetLanguage
	if bilingual {
		base += ".bilingual"
	}
	outputPath := uniqueOutput(filepath.Dir(mediaPath), base+".srt")
	if err := os.WriteFile(statePath, []byte(outputPath), 0o600); err != nil {
		return "", err
	}
	return outputPath, nil
}

func loadTranslationCheckpoint(path string, document SubtitleDocument) map[int]string {
	translations := make(map[int]string)
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &translations) != nil {
		return translations
	}
	known := make(map[int]bool, len(document.Cues))
	for _, cue := range document.Cues {
		known[cue.ID] = true
	}
	for id, text := range translations {
		if !known[id] || strings.TrimSpace(text) == "" {
			delete(translations, id)
		}
	}
	return translations
}

func saveTranslationCheckpoint(path string, translations map[int]string) error {
	data, err := json.Marshal(translations)
	if err != nil {
		return err
	}
	return writeTranslationFile(path, data)
}

func writeTranslationFile(path string, data []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}
