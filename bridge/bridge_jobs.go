package downkit

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type bridgeJobFile struct {
	Index      int    `json:"index,omitempty"`
	ID         string `json:"id,omitempty"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	OutputPath string `json:"outputPath,omitempty"`
}

type bridgeJob struct {
	ID                   string          `json:"id"`
	Title                string          `json:"title"`
	SourceURL            string          `json:"sourceUrl"`
	Status               string          `json:"status"`
	Phase                string          `json:"phase,omitempty"`
	Progress             int             `json:"progress"`
	Detail               string          `json:"detail,omitempty"`
	DownloadedBytes      int64           `json:"downloadedBytes,omitempty"`
	TotalBytes           int64           `json:"totalBytes,omitempty"`
	SpeedBytesPerSecond  int64           `json:"speedBytesPerSecond,omitempty"`
	OutputPaths          []string        `json:"outputPaths,omitempty"`
	Files                []bridgeJobFile `json:"files,omitempty"`
	Error                string          `json:"error,omitempty"`
	RequiresFreshSession bool            `json:"requiresFreshSession,omitempty"`
	CookieStoreID        string          `json:"cookieStoreId,omitempty"`
	SourceTabID          int             `json:"sourceTabId,omitempty"`
	SourceFrameID        int             `json:"sourceFrameId,omitempty"`
	PageURL              string          `json:"pageUrl,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	task                 bridgeTask
	worker               *exec.Cmd
	workerToken          string
	runID                uint64
}

func publicBridgeJob(job *bridgeJob) bridgeJob {
	return bridgeJob{
		ID: job.ID, Title: job.Title, SourceURL: job.SourceURL, Status: job.Status, Phase: job.Phase,
		Progress: job.Progress, Detail: job.Detail, DownloadedBytes: job.DownloadedBytes,
		TotalBytes: job.TotalBytes, SpeedBytesPerSecond: job.SpeedBytesPerSecond,
		OutputPaths: append([]string(nil), job.OutputPaths...),
		Files:       append([]bridgeJobFile(nil), job.Files...),
		Error:       job.Error, RequiresFreshSession: job.RequiresFreshSession, CookieStoreID: job.CookieStoreID,
		SourceTabID: job.SourceTabID, SourceFrameID: job.SourceFrameID, PageURL: job.PageURL,
		CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
}

func (s *bridgeServer) enqueueTask(task bridgeTask) (string, error) {
	jobToken, err := randomToken()
	if err != nil {
		return "", err
	}
	jobID := jobToken[:12]
	now := time.Now()

	s.mu.Lock()
	for token, item := range s.pending {
		if now.After(item.expiresAt) {
			delete(s.pending, token)
		}
	}
	s.jobs[jobID] = &bridgeJob{
		ID: jobID, Title: task.Title, SourceURL: task.URL, Status: "queued", Phase: "resolving",
		Detail: "等待启动", CreatedAt: now, UpdatedAt: now, CookieStoreID: task.CookieStoreID,
		SourceTabID: task.SourceTabID, SourceFrameID: task.SourceFrameID, PageURL: task.PageURL, task: task,
	}
	_ = s.persistJobsLocked()
	s.mu.Unlock()
	if err := s.startJob(jobID); err != nil {
		s.mu.Lock()
		delete(s.jobs, jobID)
		_ = s.persistJobsLocked()
		s.mu.Unlock()
		return "", err
	}
	return jobID, nil
}

func (s *bridgeServer) startJob(jobID string) error {
	workerToken, err := randomToken()
	if err != nil {
		return err
	}
	now := time.Now()
	s.mu.Lock()
	job := s.jobs[jobID]
	if job == nil {
		s.mu.Unlock()
		return errors.New("task not found")
	}
	if job.RequiresFreshSession {
		s.mu.Unlock()
		return errors.New("任务依赖已失效的浏览器会话，请回到原页面重新提交")
	}
	resuming := job.Status == "paused" || job.Status == "failed"
	job.runID++
	runID := job.runID
	job.Status = "queued"
	if resuming {
		job.Detail = "正在从检查点恢复"
	} else {
		job.Phase = "resolving"
		job.Progress = 0
		job.Detail = "等待启动"
		job.DownloadedBytes = 0
		job.TotalBytes = 0
	}
	job.Error = ""
	job.SpeedBytesPerSecond = 0
	job.UpdatedAt = now
	job.workerToken = workerToken
	s.pending[workerToken] = pendingBridgeTask{task: job.task, config: s.config, jobID: jobID, expiresAt: now.Add(time.Minute)}
	_ = s.persistJobsLocked()
	s.mu.Unlock()

	command, err := workerCommand(workerToken)
	if err == nil {
		err = command.Start()
	}
	if err != nil {
		s.mu.Lock()
		delete(s.pending, workerToken)
		if current := s.jobs[jobID]; current != nil && current.runID == runID {
			current.Status = "failed"
			current.Error = "无法启动下载进程"
			current.UpdatedAt = time.Now()
			_ = s.persistJobsLocked()
		}
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	if current := s.jobs[jobID]; current != nil && current.runID == runID {
		current.worker = command
	}
	s.mu.Unlock()
	go s.waitForWorker(jobID, runID, command)
	return nil
}

type refreshedJobCredentials struct {
	UserAgent    string          `json:"userAgent"`
	MediaCookies []browserCookie `json:"mediaCookies"`
	PageCookies  []browserCookie `json:"pageCookies"`
}

func applyRefreshedJobCredentials(task *bridgeTask, refreshed refreshedJobCredentials) error {
	userAgent := strings.TrimSpace(refreshed.UserAgent)
	if strings.ContainsAny(userAgent, "\r\n") {
		return errors.New("invalid user agent")
	}
	if userAgent != "" {
		task.UserAgent = userAgent
	}
	if len(refreshed.MediaCookies) > 0 {
		task.MediaCookies = sanitizeBrowserCookies(refreshed.MediaCookies)
	}
	if len(refreshed.PageCookies) > 0 {
		task.PageCookies = sanitizeBrowserCookies(refreshed.PageCookies)
	}
	return nil
}

func decodeRefreshedJobCredentials(response http.ResponseWriter, request *http.Request) (refreshedJobCredentials, error) {
	var refreshed refreshedJobCredentials
	if request.Body == nil || request.ContentLength == 0 {
		return refreshed, nil
	}
	if err := decodeBridgeJSON(response, request, &refreshed); err != nil {
		return refreshed, err
	}
	return refreshed, nil
}

func (s *bridgeServer) waitForWorker(jobID string, runID uint64, command *exec.Cmd) {
	err := command.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil || job.runID != runID {
		return
	}
	job.worker = nil
	if job.Status == "queued" || job.Status == "running" {
		job.Status = "failed"
		job.Detail = "下载已停止"
		job.Error = "下载进程意外退出"
		if err != nil {
			job.Error = err.Error()
		}
		job.SpeedBytesPerSecond = 0
		job.UpdatedAt = time.Now()
		_ = s.persistJobsLocked()
	}
}

func (s *bridgeServer) listJobs() []bridgeJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]bridgeJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, publicBridgeJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	if len(jobs) > 50 {
		jobs = jobs[:50]
	}
	return jobs
}

func (s *bridgeServer) setJobStatus(jobID, status, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return errors.New("task not found")
	}
	if job.Status == "paused" {
		return nil
	}
	switch status {
	case "running", "completed", "failed":
	default:
		return errors.New("invalid task status")
	}
	job.Status = status
	job.Error = strings.TrimSpace(message)
	if status == "running" && job.Detail == "等待启动" {
		job.Detail = "正在准备下载"
	}
	if status == "completed" {
		job.Phase = "completed"
		job.Progress = 100
		job.Detail = "下载完成"
	}
	if status == "failed" {
		job.Detail = "下载失败"
	}
	job.SpeedBytesPerSecond = 0
	job.UpdatedAt = time.Now()
	_ = s.persistJobsLocked()
	return nil
}

func (s *bridgeServer) setJobProgress(jobID, phase string, progress int, detail string, downloaded, total, speedBytesPerSecond int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return errors.New("task not found")
	}
	if job.Status != "running" && job.Status != "queued" {
		return nil
	}
	phase = strings.TrimSpace(phase)
	if phase != "" {
		switch phase {
		case "resolving", "downloading", "processing":
			job.Phase = phase
		default:
			return errors.New("invalid task phase")
		}
	}
	if job.Phase == "" {
		job.Phase = "downloading"
	}
	job.Progress = min(max(progress, 0), 100)
	job.Detail = strings.TrimSpace(detail)
	job.DownloadedBytes = max(downloaded, 0)
	job.TotalBytes = max(total, 0)
	job.SpeedBytesPerSecond = max(speedBytesPerSecond, 0)
	job.UpdatedAt = time.Now()
	_ = s.persistJobsLocked()
	return nil
}

func validatedOutputFile(outputDir, candidate string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(outputDir))
	if err != nil {
		return "", errors.New("下载目录无效")
	}
	path, err := filepath.Abs(strings.TrimSpace(candidate))
	if err != nil || strings.TrimSpace(candidate) == "" {
		return "", errors.New("文件路径无效")
	}
	resolvedRoot := root
	if evaluated, evaluateErr := filepath.EvalSymlinks(root); evaluateErr == nil {
		resolvedRoot = evaluated
	}
	resolvedPath := path
	if evaluated, evaluateErr := filepath.EvalSymlinks(path); evaluateErr == nil {
		resolvedPath = evaluated
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("拒绝打开下载目录外的文件")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("下载文件不存在")
	}
	return path, nil
}

func (s *bridgeServer) addJobOutput(jobID, candidate string) error {
	return s.addJobOutputFile(jobID, candidate, 0, "")
}

func (s *bridgeServer) setJobFiles(jobID string, files []bridgeJobFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return errors.New("task not found")
	}
	existing := make(map[string]bridgeJobFile, len(job.Files))
	for _, file := range job.Files {
		key := strings.ToLower(strings.TrimSpace(file.ID))
		if key == "" {
			key = fmt.Sprintf("#%d", file.Index)
		}
		existing[key] = file
	}
	merged := make([]bridgeJobFile, 0, len(files))
	for _, file := range files {
		file.Title = strings.TrimSpace(file.Title)
		file.ID = strings.TrimSpace(file.ID)
		if file.Index <= 0 || file.Title == "" {
			continue
		}
		file.Status = "pending"
		key := strings.ToLower(file.ID)
		if key == "" {
			key = fmt.Sprintf("#%d", file.Index)
		}
		if previous, ok := existing[key]; ok && previous.OutputPath != "" {
			file.OutputPath = previous.OutputPath
			file.Status = "completed"
		}
		merged = append(merged, file)
	}
	if len(merged) == 0 {
		return errors.New("playlist has no valid files")
	}
	job.Files = merged
	job.UpdatedAt = time.Now()
	_ = s.persistJobsLocked()
	return nil
}

func (s *bridgeServer) addJobOutputFile(jobID, candidate string, index int, itemID string) error {
	s.mu.Lock()
	job := s.jobs[jobID]
	outputDir := s.config.OutputDir
	s.mu.Unlock()
	if job == nil {
		return errors.New("task not found")
	}
	path, err := validatedOutputFile(outputDir, candidate)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job = s.jobs[jobID]
	if job == nil {
		return errors.New("task not found")
	}
	for _, existing := range job.OutputPaths {
		if strings.EqualFold(existing, path) {
			return nil
		}
	}
	job.OutputPaths = append(job.OutputPaths, path)
	matched := false
	for fileIndex := range job.Files {
		file := &job.Files[fileIndex]
		if index > 0 && file.Index == index || itemID != "" && strings.EqualFold(file.ID, itemID) {
			file.OutputPath = path
			file.Status = "completed"
			matched = true
			break
		}
	}
	if !matched {
		job.Files = append(job.Files, bridgeJobFile{
			Index: index, ID: strings.TrimSpace(itemID), Title: filepath.Base(path), Status: "completed", OutputPath: path,
		})
	}
	job.UpdatedAt = time.Now()
	_ = s.persistJobsLocked()
	return nil
}

func (s *bridgeServer) openJobOutput(jobID string, reveal bool, outputPath string) error {
	s.mu.Lock()
	job := s.jobs[jobID]
	outputDir := s.config.OutputDir
	var candidate string
	if job != nil {
		if outputPath == "" && len(job.OutputPaths) > 0 {
			candidate = job.OutputPaths[0]
		} else {
			for _, registered := range job.OutputPaths {
				if strings.EqualFold(registered, outputPath) {
					candidate = registered
					break
				}
			}
		}
	}
	s.mu.Unlock()
	if job == nil {
		return errors.New("task not found")
	}
	if candidate == "" {
		return errors.New("download file not found in task")
	}
	path, err := validatedOutputFile(outputDir, candidate)
	if err != nil {
		return err
	}
	if reveal {
		return platformRevealFile(path)
	}
	return platformOpenFile(path)
}

func (s *bridgeServer) pauseJob(jobID string) error {
	s.mu.Lock()
	job := s.jobs[jobID]
	if job == nil {
		s.mu.Unlock()
		return errors.New("task not found")
	}
	if job.Status != "running" && job.Status != "queued" {
		s.mu.Unlock()
		return errors.New("task cannot be paused")
	}
	command := job.worker
	delete(s.pending, job.workerToken)
	job.workerToken = ""
	job.worker = nil
	job.Status = "paused"
	job.Detail = "已暂停，可从检查点继续"
	job.SpeedBytesPerSecond = 0
	job.UpdatedAt = time.Now()
	_ = s.persistJobsLocked()
	s.mu.Unlock()
	if command != nil && command.Process != nil {
		_ = stopProcess(command)
	}
	return nil
}

func (s *bridgeServer) deleteJob(jobID string, deleteFiles bool) error {
	s.mu.Lock()
	job := s.jobs[jobID]
	if job == nil {
		s.mu.Unlock()
		return errors.New("task not found")
	}
	command := job.worker
	delete(s.pending, job.workerToken)
	outputDir := s.config.OutputDir
	outputs := append([]string(nil), job.OutputPaths...)
	job.runID++
	job.worker = nil
	job.workerToken = ""
	s.mu.Unlock()
	if command != nil && command.Process != nil {
		_ = stopProcess(command)
	}
	if deleteFiles {
		var failures []string
		var remaining []string
		for _, candidate := range outputs {
			if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
				continue
			}
			path, err := validatedOutputFile(outputDir, candidate)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
					continue
				}
				failures = append(failures, filepath.Base(candidate)+": "+err.Error())
				remaining = append(remaining, candidate)
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, filepath.Base(path)+": "+err.Error())
				remaining = append(remaining, candidate)
			}
		}
		if len(failures) > 0 {
			message := fmt.Sprintf("部分文件删除失败：%s", strings.Join(failures, "；"))
			s.mu.Lock()
			if current := s.jobs[jobID]; current != nil {
				current.OutputPaths = remaining
				for index := range current.Files {
					file := &current.Files[index]
					if file.OutputPath == "" {
						continue
					}
					kept := false
					for _, path := range remaining {
						if strings.EqualFold(path, file.OutputPath) {
							kept = true
							break
						}
					}
					if !kept {
						file.OutputPath = ""
						file.Status = "deleted"
					}
				}
				current.Status = "failed"
				current.Detail = "部分文件删除失败"
				current.Error = message
				current.SpeedBytesPerSecond = 0
				current.UpdatedAt = time.Now()
				_ = s.persistJobsLocked()
			}
			s.mu.Unlock()
			return errors.New(message)
		}
	}
	s.mu.Lock()
	delete(s.jobs, jobID)
	_ = s.persistJobsLocked()
	s.mu.Unlock()
	return nil
}

func (s *bridgeServer) handleJobs(response http.ResponseWriter, request *http.Request) {
	s.allowExtension(response, request)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/v1/jobs"), "/")
	if path == "" && request.Method == http.MethodGet {
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "jobs": s.listJobs()})
		return
	}
	if path == "clear" && request.Method == http.MethodPost {
		s.mu.Lock()
		for id, job := range s.jobs {
			if job.Status == "completed" || job.Status == "failed" {
				delete(s.jobs, id)
			}
		}
		_ = s.persistJobsLocked()
		s.mu.Unlock()
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		response.WriteHeader(http.StatusNotFound)
		return
	}
	jobID, action := parts[0], parts[1]
	if action == "progress" && request.Method == http.MethodPost {
		var update struct {
			Phase               string `json:"phase"`
			Progress            int    `json:"progress"`
			Detail              string `json:"detail"`
			DownloadedBytes     int64  `json:"downloadedBytes"`
			TotalBytes          int64  `json:"totalBytes"`
			SpeedBytesPerSecond int64  `json:"speedBytesPerSecond"`
		}
		if decodeBridgeJSON(response, request, &update) != nil {
			return
		}
		if err := s.setJobProgress(jobID, update.Phase, update.Progress, update.Detail, update.DownloadedBytes, update.TotalBytes, update.SpeedBytesPerSecond); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "output" && request.Method == http.MethodPost {
		var update struct {
			Path  string `json:"path"`
			Index int    `json:"index"`
			ID    string `json:"id"`
		}
		if decodeBridgeJSON(response, request, &update) != nil {
			return
		}
		if err := s.addJobOutputFile(jobID, update.Path, update.Index, update.ID); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "files" && request.Method == http.MethodPost {
		var update struct {
			Files []bridgeJobFile `json:"files"`
		}
		if decodeBridgeJSON(response, request, &update) != nil {
			return
		}
		if err := s.setJobFiles(jobID, update.Files); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "state" && request.Method == http.MethodPost {
		var update struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if decodeBridgeJSON(response, request, &update) != nil {
			return
		}
		if err := s.setJobStatus(jobID, update.Status, update.Error); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "retry" && request.Method == http.MethodPost {
		refreshed, err := decodeRefreshedJobCredentials(response, request)
		if err != nil {
			return
		}
		s.mu.Lock()
		job := s.jobs[jobID]
		canRetry := job != nil && job.Status == "failed"
		if canRetry {
			if err := applyRefreshedJobCredentials(&job.task, refreshed); err != nil {
				s.mu.Unlock()
				writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			job.RequiresFreshSession = false
			_ = s.persistJobsLocked()
		}
		s.mu.Unlock()
		if job == nil {
			writeJSON(response, http.StatusNotFound, map[string]any{"ok": false, "error": "task not found"})
			return
		}
		if !canRetry {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": "task cannot be retried"})
			return
		}
		if err := s.startJob(jobID); err != nil {
			writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot retry task"})
			return
		}
		writeJSON(response, http.StatusAccepted, map[string]any{"ok": true, "taskId": jobID})
		return
	}
	if action == "pause" && request.Method == http.MethodPost {
		if err := s.pauseJob(jobID); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if action == "resume" && request.Method == http.MethodPost {
		refreshed, err := decodeRefreshedJobCredentials(response, request)
		if err != nil {
			return
		}
		s.mu.Lock()
		job := s.jobs[jobID]
		canResume := job != nil && (job.Status == "paused" || job.Status == "needs-session")
		if canResume {
			if err := applyRefreshedJobCredentials(&job.task, refreshed); err != nil {
				s.mu.Unlock()
				writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			job.RequiresFreshSession = false
			_ = s.persistJobsLocked()
		}
		s.mu.Unlock()
		if !canResume {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": "task cannot be resumed"})
			return
		}
		if err := s.startJob(jobID); err != nil {
			writeJSON(response, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot resume task"})
			return
		}
		writeJSON(response, http.StatusAccepted, map[string]any{"ok": true, "taskId": jobID})
		return
	}
	if action == "delete" && request.Method == http.MethodPost {
		var options struct {
			DeleteFiles bool `json:"deleteFiles"`
		}
		if request.Body != nil && request.ContentLength != 0 {
			if decodeBridgeJSON(response, request, &options) != nil {
				return
			}
		}
		if err := s.deleteJob(jobID, options.DeleteFiles); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if (action == "open" || action == "reveal") && request.Method == http.MethodPost {
		var options struct {
			OutputPath string `json:"outputPath"`
		}
		if request.Body != nil && request.ContentLength != 0 {
			if decodeBridgeJSON(response, request, &options) != nil {
				return
			}
		}
		if err := s.openJobOutput(jobID, action == "reveal", options.OutputPath); err != nil {
			writeJSON(response, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"ok": true})
		return
	}
	response.WriteHeader(http.StatusNotFound)
}

func openDirectoryCommand(path string) *exec.Cmd {
	return platformOpenDirectoryCommand(path)
}
