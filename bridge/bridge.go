package downkit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	bridgeAddress  = "127.0.0.1:17891"
	bridgeBaseURL  = "http://" + bridgeAddress
	nativeHostName = "com.downkit.bridge"
	maxBridgeBody  = 256 << 10
)

type bridgeState struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

type bridgeTask struct {
	URL           string            `json:"url"`
	Title         string            `json:"title"`
	Referer       string            `json:"referer"`
	Origin        string            `json:"origin"`
	UserAgent     string            `json:"userAgent"`
	Quality       string            `json:"quality"`
	Playlist      string            `json:"playlist"`
	ResolvePage   bool              `json:"resolvePage"`
	MediaHeaders  map[string]string `json:"mediaHeaders"`
	PageHeaders   map[string]string `json:"pageHeaders"`
	MediaCookies  []browserCookie   `json:"mediaCookies"`
	PageCookies   []browserCookie   `json:"pageCookies"`
	CookieStoreID string            `json:"cookieStoreId,omitempty"`
	SourceTabID   int               `json:"sourceTabId,omitempty"`
	SourceFrameID int               `json:"sourceFrameId,omitempty"`
	PageURL       string            `json:"pageUrl,omitempty"`
}

type pendingBridgeTask struct {
	task      bridgeTask
	config    bridgeConfig
	jobID     string
	expiresAt time.Time
}

type workerPayload struct {
	Task   bridgeTask   `json:"task"`
	Config bridgeConfig `json:"config"`
	JobID  string       `json:"jobId"`
}

type bridgeServer struct {
	state        bridgeState
	mu           sync.Mutex
	pending      map[string]pendingBridgeTask
	jobs         map[string]*bridgeJob
	jobStorePath string
	config       bridgeConfig
	tools        *toolRegistry
	componentMu  sync.Mutex
	httpServer   *http.Server
	restart      func()
}

func bridgeStatePath() (string, error) {
	return bridgeDataPath("bridge.json")
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func writeBridgeState(state bridgeState) error {
	path, err := bridgeStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func readBridgeState() (bridgeState, error) {
	path, err := bridgeStatePath()
	if err != nil {
		return bridgeState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return bridgeState{}, err
	}
	var state bridgeState
	if err := json.Unmarshal(data, &state); err != nil {
		return bridgeState{}, err
	}
	if state.BaseURL != bridgeBaseURL || len(state.Token) < 32 {
		return bridgeState{}, errors.New("invalid bridge state")
	}
	return state, nil
}

func runBridge() error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	jobStorePath, err := bridgeJobsPath()
	if err != nil {
		return err
	}
	jobs, loadErr := loadBridgeJobs(jobStorePath)
	if loadErr != nil {
		fmt.Fprintln(os.Stderr, "警告：无法读取任务记录：", loadErr)
		jobs = make(map[string]*bridgeJob)
	}
	server := &bridgeServer{
		state:        bridgeState{BaseURL: bridgeBaseURL, Token: token, PID: os.Getpid()},
		pending:      make(map[string]pendingBridgeTask),
		jobs:         jobs,
		jobStorePath: jobStorePath,
		config:       loadBridgeConfig(),
		tools:        newDesktopToolRegistry(),
	}
	server.mu.Lock()
	_ = server.persistJobsLocked()
	server.mu.Unlock()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ping", server.handlePing)
	mux.HandleFunc("/v1/tasks", server.handleTasks)
	mux.HandleFunc("/v1/playlist/probe", server.handlePlaylistProbe)
	mux.HandleFunc("/v1/worker/", server.handleWorker)
	mux.HandleFunc("/v1/environment", server.handleEnvironment)
	mux.HandleFunc("/v1/config", server.handleConfig)
	mux.HandleFunc("/v1/tools", server.handleTools)
	mux.HandleFunc("/v1/tools/yt-dlp/install", server.handleInstallYTDLP)
	mux.HandleFunc("/v1/restart", server.handleRestart)
	mux.HandleFunc("/v1/jobs", server.handleJobs)
	mux.HandleFunc("/v1/jobs/", server.handleJobs)
	mux.HandleFunc("/v1/open-output", server.handleOpenOutput)

	httpServer := &http.Server{
		Addr:              bridgeAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	server.httpServer = httpServer
	restartRequested := make(chan struct{}, 1)
	server.restart = func() {
		select {
		case restartRequested <- struct{}{}:
		default:
		}
	}
	if err := writeBridgeState(server.state); err != nil {
		return err
	}
	err = httpServer.ListenAndServe()
	select {
	case <-restartRequested:
		command, commandErr := bridgeCommand()
		if commandErr != nil {
			return commandErr
		}
		return command.Start()
	default:
		return err
	}
}

func (s *bridgeServer) authorized(request *http.Request) bool {
	provided := request.Header.Get("X-DownKit-Token")
	return len(provided) == len(s.state.Token) && subtle.ConstantTimeCompare([]byte(provided), []byte(s.state.Token)) == 1
}

func (s *bridgeServer) allowExtension(response http.ResponseWriter, request *http.Request) {
	origin := request.Header.Get("Origin")
	if strings.HasPrefix(origin, "chrome-extension://") {
		response.Header().Set("Access-Control-Allow-Origin", origin)
		response.Header().Set("Vary", "Origin")
	}
	response.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-DownKit-Token")
	response.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func decodeBridgeJSON(response http.ResponseWriter, request *http.Request, value any) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxBridgeBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid request"})
		return err
	}
	return nil
}

func (s *bridgeServer) handlePing(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodGet || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "pid": s.state.PID})
}

func (s *bridgeServer) handleTasks(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var task bridgeTask
	if decodeBridgeJSON(response, request, &task) != nil {
		return
	}
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	if _, err := optionsFromBridgeTask(task, config); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	jobID, err := s.enqueueTask(task)
	if err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot start worker"})
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"ok": true, "taskId": jobID})
}

func (s *bridgeServer) handleWorker(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(request.URL.Path, "/v1/worker/")
	if len(token) != 64 || strings.Contains(token, "/") {
		response.WriteHeader(http.StatusNotFound)
		return
	}
	s.mu.Lock()
	item, ok := s.pending[token]
	delete(s.pending, token)
	s.mu.Unlock()
	if !ok || time.Now().After(item.expiresAt) {
		response.WriteHeader(http.StatusGone)
		return
	}
	s.mu.Lock()
	if job := s.jobs[item.jobID]; job != nil {
		job.Status = "running"
		job.UpdatedAt = time.Now()
		_ = s.persistJobsLocked()
	}
	s.mu.Unlock()
	writeJSON(response, http.StatusOK, workerPayload{Task: item.task, Config: item.config, JobID: item.jobID})
}

func optionsFromBridgeTask(task bridgeTask, config bridgeConfig) (options, error) {
	o := options{
		sourceURL: task.URL, title: task.Title, referer: task.Referer, origin: task.Origin,
		userAgent: task.UserAgent, proxy: config.Proxy, outputDir: config.OutputDir,
		ffmpegPath: config.FFmpegPath, ytDLPPath: config.YTDLPPath,
		playlistMode: task.Playlist, concurrent: config.Concurrent, resolvePage: task.ResolvePage,
	}
	if o.userAgent == "" {
		o.userAgent = defaultUA
	}
	if o.playlistMode == "" {
		o.playlistMode = "ask"
	}
	mediaHeaders, err := sanitizeHeaderMap(task.MediaHeaders)
	if err != nil {
		return o, err
	}
	pageHeaders, err := sanitizeHeaderMap(task.PageHeaders)
	if err != nil {
		return o, err
	}
	o.requestHeaders = mediaHeaders
	o.pageRequestHeaders = pageHeaders
	o.mediaCookies = sanitizeBrowserCookies(task.MediaCookies)
	o.pageCookies = sanitizeBrowserCookies(task.PageCookies)
	if task.ResolvePage && len(pageHeaders) > 0 {
		o.requestHeaders = pageHeaders
	}
	quality := task.Quality
	if quality == "" {
		quality = config.Quality
	}
	if quality != "" {
		if err := setQuality(&o, quality); err != nil {
			return o, err
		}
	}
	return normalizeOptions(o)
}

func sanitizeHeaderMap(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return parseRequestHeaders(string(data))
}

func sanitizeBrowserCookies(values []browserCookie) []browserCookie {
	if len(values) > 5000 {
		values = values[:5000]
	}
	result := make([]browserCookie, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, cookie := range values {
		cookie.Name = strings.TrimSpace(cookie.Name)
		cookie.Domain = strings.ToLower(strings.TrimSpace(cookie.Domain))
		cookie.Path = strings.TrimSpace(cookie.Path)
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		if !strings.HasPrefix(cookie.Path, "/") {
			cookie.Path = "/" + cookie.Path
		}
		if cookie.Name == "" || cookie.Domain == "" ||
			strings.ContainsAny(cookie.Name+cookie.Value+cookie.Domain+cookie.Path, "\r\n\t") ||
			strings.Contains(cookie.Domain, "://") {
			continue
		}
		key := cookie.Name + "\n" + cookie.Domain + "\n" + cookie.Path
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, cookie)
	}
	return result
}

func runWorker(workerToken string) error {
	if len(workerToken) != 64 {
		return errors.New("invalid worker token")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(bridgeBaseURL + "/v1/worker/" + workerToken)
	if err != nil {
		return fmt.Errorf("cannot obtain task from bridge: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge returned %s", response.Status)
	}
	var payload workerPayload
	if err := json.NewDecoder(io.LimitReader(response.Body, maxBridgeBody)).Decode(&payload); err != nil {
		return err
	}
	reportJobState(payload.JobID, "running", "")
	opts, err := optionsFromBridgeTask(payload.Task, payload.Config)
	if err != nil {
		reportJobState(payload.JobID, "failed", err.Error())
		return err
	}
	if opts.playlistMode == "ask" {
		opts.playlistMode = "single"
	}
	opts.jobID = payload.JobID
	beginJobReporting(payload.JobID)
	err = runWithOptions(opts, nil)
	if err != nil {
		reportJobState(payload.JobID, "failed", err.Error())
		return err
	}
	reportJobState(payload.JobID, "completed", "")
	return nil
}

func reportJobState(jobID, status, message string) {
	state, err := readBridgeState()
	if err != nil {
		return
	}
	body, _ := json.Marshal(map[string]string{"status": status, "error": message})
	request, _ := http.NewRequest(http.MethodPost, state.BaseURL+"/v1/jobs/"+jobID+"/state", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-DownKit-Token", state.Token)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func (s *bridgeServer) handleEnvironment(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodGet || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "environment": inspectBridgeEnvironment(config)})
}

func (s *bridgeServer) handleTools(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodGet || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	registry := s.tools
	if registry == nil {
		registry = newDesktopToolRegistry()
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"ok":          true,
		"version":     1,
		"generatedAt": time.Now(),
		"tools":       registry.snapshots(request.Context(), config),
	})
}

func (s *bridgeServer) handleInstallYTDLP(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	s.componentMu.Lock()
	defer s.componentMu.Unlock()
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	path, err := installYTDLPComponent(request.Context(), config.Proxy)
	if err != nil {
		writeJSON(response, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *bridgeServer) handleRestart(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	s.mu.Lock()
	unfinished := 0
	for _, job := range s.jobs {
		if job.Status == "queued" || job.Status == "running" {
			unfinished++
		}
	}
	s.mu.Unlock()
	if unfinished > 0 {
		writeJSON(response, http.StatusConflict, map[string]any{
			"ok": false, "error": fmt.Sprintf("仍有 %d 个正在执行的下载任务，请先暂停后再重启", unfinished),
		})
		return
	}
	if s.restart == nil || s.httpServer == nil {
		writeJSON(response, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "当前 Bridge 不支持重启"})
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"ok": true, "restarting": true})
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.restart()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(ctx)
	}()
}

func (s *bridgeServer) handleConfig(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	if request.Method == http.MethodGet {
		s.mu.Lock()
		config := s.config
		s.mu.Unlock()
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "config": config})
		return
	}
	if request.Method == http.MethodPut {
		var config bridgeConfig
		if decodeBridgeJSON(response, request, &config) != nil {
			return
		}
		normalized, err := normalizeBridgeConfig(config)
		if err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := saveBridgeConfig(normalized); err != nil {
			writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "无法保存配置"})
			return
		}
		s.mu.Lock()
		s.config = normalized
		s.mu.Unlock()
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "config": normalized})
		return
	}
	response.WriteHeader(http.StatusMethodNotAllowed)
}

func (s *bridgeServer) handleOpenOutput(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost || !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	s.mu.Lock()
	path := s.config.OutputDir
	s.mu.Unlock()
	if err := os.MkdirAll(path, 0o755); err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "无法打开下载目录"})
		return
	}
	if err := openDirectoryCommand(path).Start(); err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "无法打开下载目录"})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true})
}

type nativeRequest struct {
	Command string `json:"command"`
}

func runNativeHost() error {
	for {
		message, err := readNativeMessage(os.Stdin)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var request nativeRequest
		if err := json.Unmarshal(message, &request); err != nil {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": "invalid request"})
			continue
		}
		if request.Command != "ensure_bridge" && request.Command != "status" {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": "unknown command"})
			continue
		}
		state, err := ensureBridge()
		if err != nil {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": err.Error()})
			continue
		}
		if err := writeNativeMessage(os.Stdout, map[string]any{"ok": true, "baseUrl": state.BaseURL, "token": state.Token, "pid": state.PID}); err != nil {
			return err
		}
	}
}

func ensureBridge() (bridgeState, error) {
	if state, err := readBridgeState(); err == nil && pingBridge(state) == nil {
		return state, nil
	}
	command, err := bridgeCommand()
	if err != nil {
		return bridgeState{}, err
	}
	if err := command.Start(); err != nil {
		return bridgeState{}, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		state, err := readBridgeState()
		if err == nil && pingBridge(state) == nil {
			return state, nil
		}
	}
	return bridgeState{}, errors.New("DownKit Bridge did not start")
}

func pingBridge(state bridgeState) error {
	request, _ := http.NewRequest(http.MethodGet, state.BaseURL+"/v1/ping", nil)
	request.Header.Set("X-DownKit-Token", state.Token)
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge returned %s", response.Status)
	}
	return nil
}

func currentExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func readNativeMessage(reader io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 || size > maxBridgeBody {
		return nil, errors.New("invalid native message size")
	}
	message := make([]byte, size)
	_, err := io.ReadFull(reader, message)
	return message, err
}

func writeNativeMessage(writer io.Writer, value any) error {
	message, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(message))); err != nil {
		return err
	}
	_, err = writer.Write(message)
	return err
}

// Platform-specific implementations control whether Bridge is hidden and Worker gets a console.
func bridgeCommand() (*exec.Cmd, error)             { return newBridgeCommand() }
func workerCommand(token string) (*exec.Cmd, error) { return newWorkerCommand(token) }
