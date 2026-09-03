package downkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const gemmaTermsURL = "https://ai.google.dev/gemma/terms"

type translationModelDescriptor struct {
	ID       string
	Filename string
	Size     int64
	SHA256   string
	URL      string
}

var translationModelCatalog = []translationModelDescriptor{{
	ID:       "translategemma-4b-q4-k-m",
	Filename: "translategemma-4b_Q4_K_M.gguf",
	Size:     2489909312,
	SHA256:   "526747309109c016db547c6fc1c7b0c9c286b5e7a7556827b5419fd9543a09cd",
	URL:      "https://huggingface.co/SandLogicTechnologies/translategemma-4b-it-GGUF/resolve/main/translategemma-4b_Q4_K_M.gguf?download=true",
}}

func findTranslationModel(id string) (translationModelDescriptor, bool) {
	for _, model := range translationModelCatalog {
		if model.ID == id {
			return model, true
		}
	}
	return translationModelDescriptor{}, false
}

func validTranslationModelFile(path string, model translationModelDescriptor) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() == model.Size
}

func verifiedTranslationModel(path string, model translationModelDescriptor) bool {
	if !validTranslationModelFile(path, model) {
		return false
	}
	digest, err := fileSHA256(path)
	return err == nil && digest == model.SHA256
}

func translationModelDownloadPath(model translationModelDescriptor) (string, error) {
	directory, err := bridgeDataPath(filepath.Join("models", "translation"))
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, model.Filename), nil
}

func installedTranslationModelPath(config bridgeConfig, model translationModelDescriptor) string {
	if filepath.Base(config.TranslationModel) == model.Filename && validTranslationModelFile(config.TranslationModel, model) {
		return config.TranslationModel
	}
	if path, err := translationModelDownloadPath(model); err == nil && validTranslationModelFile(path, model) {
		return path
	}
	return ""
}

func translationModelViews(config bridgeConfig) []whisperModelView {
	views := make([]whisperModelView, 0, len(translationModelCatalog))
	for _, model := range translationModelCatalog {
		path := installedTranslationModelPath(config, model)
		views = append(views, whisperModelView{
			ID: model.ID, SizeBytes: model.Size, Language: "55 种语言（含中/英/日）", Variant: "4B · Q4_K_M",
			Recommended: true, Installed: path != "", Active: sameLocalPath(path, config.TranslationModel), Path: path,
			Source: "TranslateGemma 社区 GGUF 量化", License: "Gemma", LicenseURL: gemmaTermsURL,
			RequiresLicenseAcceptance: true,
		})
	}
	return views
}

func downloadTranslationModel(ctx context.Context, client *http.Client, model translationModelDescriptor, target string) error {
	if verifiedTranslationModel(target, model) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	partial := target + ".part"
	resumeAt := int64(0)
	if info, err := os.Stat(partial); err == nil && !info.IsDir() && info.Size() < model.Size {
		resumeAt = info.Size()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, model.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "DownKit/"+bridgeVersion)
	if resumeAt > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeAt))
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	appendDownload := resumeAt > 0 && response.StatusCode == http.StatusPartialContent
	if response.StatusCode != http.StatusOK && !appendDownload {
		return fmt.Errorf("下载 %s 失败：HTTP %d", model.Filename, response.StatusCode)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendDownload {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		resumeAt = 0
	}
	file, err := os.OpenFile(partial, flags, 0o600)
	if err != nil {
		return err
	}
	remaining := model.Size - resumeAt
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, remaining+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != remaining {
		return fmt.Errorf("翻译模型下载不完整：期望 %d 字节，实际 %d 字节", remaining, written)
	}
	if !verifiedTranslationModel(partial, model) {
		return errors.New("翻译模型 SHA-256 校验失败")
	}
	return replaceFile(partial, target)
}

func installTranslationModelComponent(ctx context.Context, proxy, id string, accepted bool, config bridgeConfig) (string, error) {
	if !accepted {
		return "", errors.New("下载 TranslateGemma 前必须确认已阅读并接受 Gemma 使用条款")
	}
	model, ok := findTranslationModel(id)
	if !ok {
		return "", fmt.Errorf("不支持的字幕翻译模型：%s", id)
	}
	if path := installedTranslationModelPath(config, model); path != "" && verifiedTranslationModel(path, model) {
		return path, nil
	}
	target, err := translationModelDownloadPath(model)
	if err != nil {
		return "", err
	}
	transport, err := proxyHTTPTransport(proxy)
	if err != nil {
		return "", err
	}
	client := &http.Client{Transport: transport}
	started := time.Now()
	fmt.Fprintf(consoleOut, "翻译模型下载：开始 id=%s size=%d\n", model.ID, model.Size)
	if err := downloadTranslationModel(ctx, client, model, target); err != nil {
		fmt.Fprintf(consoleErr, "翻译模型下载：失败 id=%s durationMs=%d error=%v\n", model.ID, time.Since(started).Milliseconds(), err)
		return "", err
	}
	fmt.Fprintf(consoleOut, "翻译模型下载：完成 id=%s durationMs=%d\n", model.ID, time.Since(started).Milliseconds())
	return target, nil
}

type translationModelInstallRequest struct {
	Model         string `json:"model"`
	AcceptLicense bool   `json:"acceptLicense"`
}

func (s *bridgeServer) handleInstallTranslationModel(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var input translationModelInstallRequest
	if decodeBridgeJSON(response, request, &input) != nil {
		return
	}
	s.componentMu.Lock()
	defer s.componentMu.Unlock()
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	path, err := installTranslationModelComponent(request.Context(), config.Proxy, input.Model, input.AcceptLicense, config)
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	config.TranslationModel = path
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := saveBridgeConfig(normalized); err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "模型已下载，但无法保存配置"})
		return
	}
	s.mu.Lock()
	s.config = normalized
	s.mu.Unlock()
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "model": input.Model, "path": path})
}
