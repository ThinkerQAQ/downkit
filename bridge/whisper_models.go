package downkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const whisperModelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

type whisperModelDescriptor struct {
	ID         string
	Size       int64
	SHA256     string
	BaseURL    string
	ThirdParty bool
}

type whisperModelView struct {
	ID                        string `json:"id"`
	SizeBytes                 int64  `json:"sizeBytes"`
	Language                  string `json:"language"`
	Variant                   string `json:"variant"`
	Recommended               bool   `json:"recommended,omitempty"`
	Installed                 bool   `json:"installed"`
	Active                    bool   `json:"active"`
	Path                      string `json:"path,omitempty"`
	Source                    string `json:"source"`
	License                   string `json:"license,omitempty"`
	LicenseURL                string `json:"licenseUrl,omitempty"`
	RequiresLicenseAcceptance bool   `json:"requiresLicenseAcceptance,omitempty"`
}

func whisperModel(id string, size int64, checksum string) whisperModelDescriptor {
	return whisperModelDescriptor{ID: id, Size: size, SHA256: checksum, BaseURL: whisperModelBaseURL}
}

// This catalog mirrors the model IDs accepted by whisper.cpp's official
// download-ggml-model.sh. Sizes and SHA-256 values are pinned from the backing
// Hugging Face LFS objects so a changed or truncated model is never activated.
var whisperModelCatalog = []whisperModelDescriptor{
	whisperModel("tiny", 77691713, "be07e048e1e599ad46341c8d2a135645097a538221678b7acdd1b1919c6e1b21"),
	whisperModel("tiny.en", 77704715, "921e4cf8686fdd993dcd081a5da5b6c365bfde1162e72b08d75ac75289920b1f"),
	whisperModel("tiny-q5_1", 32152673, "818710568da3ca15689e31a743197b520007872ff9576237bda97bd1b469c3d7"),
	whisperModel("tiny.en-q5_1", 32166155, "c77c5766f1cef09b6b7d47f21b546cbddd4157886b3b5d6d4f709e91e66c7c2b"),
	whisperModel("tiny-q8_0", 43537433, "c2085835d3f50733e2ff6e4b41ae8a2b8d8110461e18821b09a15c40c42d1cca"),
	whisperModel("base", 147951465, "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe"),
	whisperModel("base.en", 147964211, "a03779c86df3323075f5e796cb2ce5029f00ec8869eee3fdfb897afe36c6d002"),
	whisperModel("base-q5_1", 59707625, "422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898"),
	whisperModel("base.en-q5_1", 59721011, "4baf70dd0d7c4247ba2b81fafd9c01005ac77c2f9ef064e00dcf195d0e2fdd2f"),
	whisperModel("base-q8_0", 81768585, "c577b9a86e7e048a0b7eada054f4dd79a56bbfa911fbdacf900ac5b567cbb7d9"),
	whisperModel("small", 487601967, "1be3a9b2063867b937e64e2ec7483364a79917e157fa98c5d94b5c1fffea987b"),
	whisperModel("small.en", 487614201, "c6138d6d58ecc8322097e0f987c32f1be8bb0a18532a3f88f734d1bbf9c41e5d"),
	whisperModel("small-q5_1", 190085487, "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb"),
	whisperModel("small.en-q5_1", 190098681, "bfdff4894dcb76bbf647d56263ea2a96645423f1669176f4844a1bf8e478ad30"),
	whisperModel("small-q8_0", 264464607, "49c8fb02b65e6049d5fa6c04f81f53b867b5ec9540406812c643f177317f779f"),
	whisperModel("medium", 1533763059, "6c14d5adee5f86394037b4e4e8b59f1673b6cee10e3cf0b11bbdbee79c156208"),
	whisperModel("medium.en", 1533774781, "cc37e93478338ec7700281a7ac30a10128929eb8f427dda2e865faa8f6da4356"),
	whisperModel("medium-q5_0", 539212467, "19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f"),
	whisperModel("medium.en-q5_0", 539225533, "76733e26ad8fe1c7a5bf7531a9d41917b2adc0f20f2e4f5531688a8c6cd88eb0"),
	whisperModel("medium-q8_0", 823369779, "42a1ffcbe4167d224232443396968db4d02d4e8e87e213d3ee2e03095dea6502"),
	whisperModel("large-v1", 3094623691, "7d99f41a10525d0206bddadd86760181fa920438b6b33237e3118ff6c83bb53d"),
	whisperModel("large-v2", 3094623691, "9a423fe4d40c82774b6af34115b8b935f34152246eb19e80e376071d3f999487"),
	whisperModel("large-v2-q5_0", 1080732091, "3a214837221e4530dbc1fe8d734f302af393eb30bd0ed046042ebf4baf70f6f2"),
	whisperModel("large-v2-q8_0", 1656129691, "fef54e6d898246a65c8285bfa83bd1807e27fadf54d5d4e81754c47634737e8c"),
	whisperModel("large-v3", 3095033483, "64d182b440b98d5203c4f9bd541544d84c605196c4f7b845dfa11fb23594d1e2"),
	whisperModel("large-v3-q5_0", 1081140203, "d75795ecff3f83b5faa89d1900604ad8c780abd5739fae406de19f23ecd98ad1"),
	whisperModel("large-v3-turbo", 1624555275, "1fc70f774d38eb169993ac391eea357ef47c88757ef72ee5943879b7e8e2bc69"),
	whisperModel("large-v3-turbo-q5_0", 574041195, "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2"),
	whisperModel("large-v3-turbo-q8_0", 874188075, "317eb69c11673c9de1e1f0d459b253999804ec71ac4c23c17ecf5fbe24e259a1"),
	{ID: "small.en-tdrz", Size: 487614184, SHA256: "ceac3ec06d1d98ef71aec665283564631055fd6129b79d8e1be4f9cc33cc54b4", BaseURL: "https://huggingface.co/akashmjn/tinydiarize-whisper.cpp/resolve/main", ThirdParty: true},
}

func findWhisperModel(id string) (whisperModelDescriptor, bool) {
	id = strings.TrimSpace(id)
	for _, model := range whisperModelCatalog {
		if model.ID == id {
			return model, true
		}
	}
	return whisperModelDescriptor{}, false
}

func (m whisperModelDescriptor) filename() string { return "ggml-" + m.ID + ".bin" }

func (m whisperModelDescriptor) url() string {
	return strings.TrimRight(m.BaseURL, "/") + "/" + m.filename()
}

func whisperModelLanguage(id string) string {
	if strings.Contains(id, ".en") {
		return "英语专用"
	}
	return "多语言（含中文）"
}

func whisperModelVariant(id string) string {
	parts := make([]string, 0, 2)
	if strings.Contains(id, "-q") {
		parts = append(parts, "量化版")
	} else {
		parts = append(parts, "标准版")
	}
	if strings.Contains(id, "-tdrz") {
		parts = append(parts, "说话人轮次")
	}
	return strings.Join(parts, " · ")
}

func sameLocalPath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func validWhisperModelFile(path string, model whisperModelDescriptor) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() == model.Size
}

func whisperModelDownloadPath(model whisperModelDescriptor) (string, error) {
	directory, err := bridgeDataPath(filepath.Join("models", "whisper"))
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, model.filename()), nil
}

func installedWhisperModelPath(config bridgeConfig, model whisperModelDescriptor) string {
	if filepath.Base(config.WhisperModel) == model.filename() && validWhisperModelFile(config.WhisperModel, model) {
		return config.WhisperModel
	}
	if path, err := whisperModelDownloadPath(model); err == nil && validWhisperModelFile(path, model) {
		return path
	}
	if client, err := findTool(config.WhisperPath, "whisper-cli"); err == nil {
		path := filepath.Join(filepath.Dir(client), "models", model.filename())
		if validWhisperModelFile(path, model) {
			return path
		}
	}
	return ""
}

func whisperModelViews(config bridgeConfig) []whisperModelView {
	views := make([]whisperModelView, 0, len(whisperModelCatalog))
	for _, model := range whisperModelCatalog {
		path := installedWhisperModelPath(config, model)
		views = append(views, whisperModelView{
			ID: model.ID, SizeBytes: model.Size, Language: whisperModelLanguage(model.ID),
			Variant: whisperModelVariant(model.ID), Recommended: model.ID == "base",
			Installed: path != "", Active: sameLocalPath(path, config.WhisperModel), Path: path,
			Source: func() string {
				if model.ThirdParty {
					return "tinydiarize 社区仓库"
				}
				return "whisper.cpp 官方模型仓库"
			}(),
		})
	}
	return views
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifiedWhisperModel(path string, model whisperModelDescriptor) bool {
	if !validWhisperModelFile(path, model) {
		return false
	}
	digest, err := fileSHA256(path)
	return err == nil && digest == model.SHA256
}

func downloadWhisperModel(ctx context.Context, client *http.Client, model whisperModelDescriptor, target string) error {
	if verifiedWhisperModel(target, model) {
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, model.url(), nil)
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
		return fmt.Errorf("下载 %s 失败：HTTP %d", model.filename(), response.StatusCode)
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
		return fmt.Errorf("模型下载不完整：期望 %d 字节，实际 %d 字节", remaining, written)
	}
	if !verifiedWhisperModel(partial, model) {
		return errors.New("模型 SHA-256 校验失败")
	}
	return replaceFile(partial, target)
}

func installWhisperModelComponent(ctx context.Context, proxy, id string, config bridgeConfig) (string, error) {
	model, ok := findWhisperModel(id)
	if !ok {
		return "", fmt.Errorf("不支持的 Whisper 模型：%s", id)
	}
	if path := installedWhisperModelPath(config, model); path != "" && verifiedWhisperModel(path, model) {
		return path, nil
	}
	target, err := whisperModelDownloadPath(model)
	if err != nil {
		return "", err
	}
	transport, err := proxyHTTPTransport(proxy)
	if err != nil {
		return "", err
	}
	client := &http.Client{Transport: transport}
	started := time.Now()
	fmt.Fprintf(consoleOut, "Whisper 模型下载：开始 id=%s size=%d\n", model.ID, model.Size)
	if err := downloadWhisperModel(ctx, client, model, target); err != nil {
		fmt.Fprintf(consoleErr, "Whisper 模型下载：失败 id=%s durationMs=%d error=%v\n", model.ID, time.Since(started).Milliseconds(), err)
		return "", err
	}
	fmt.Fprintf(consoleOut, "Whisper 模型下载：完成 id=%s durationMs=%d\n", model.ID, time.Since(started).Milliseconds())
	return target, nil
}

type whisperModelInstallRequest struct {
	Model string `json:"model"`
}

func (s *bridgeServer) handleInstallWhisperModel(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var input whisperModelInstallRequest
	if decodeBridgeJSON(response, request, &input) != nil {
		return
	}
	s.componentMu.Lock()
	defer s.componentMu.Unlock()
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	path, err := installWhisperModelComponent(request.Context(), config.Proxy, input.Model, config)
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	config.WhisperModel = path
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
