package downkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const bridgeJobStoreVersion = 1

type persistedBridgeJobs struct {
	Version int                  `json:"version"`
	Jobs    []persistedBridgeJob `json:"jobs"`
}

type persistedBridgeJob struct {
	Job                  bridgeJob  `json:"job"`
	Task                 bridgeTask `json:"task"`
	RequiresFreshSession bool       `json:"requiresFreshSession,omitempty"`
}

func bridgeJobsPath() (string, error) {
	return bridgeDataPath("jobs.json")
}

// persistentBridgeTask deliberately drops browser credentials. A resumed public
// download can continue from its work directory; a protected download must be
// resubmitted by the extension with a fresh browser session.
func persistentBridgeTask(task bridgeTask) bridgeTask {
	task.MediaCookies = nil
	task.PageCookies = nil
	task.MediaHeaders = persistentHeaderMap(task.MediaHeaders)
	task.PageHeaders = persistentHeaderMap(task.PageHeaders)
	return task
}

func bridgeTaskHasCredentials(task bridgeTask) bool {
	if len(task.MediaCookies) > 0 || len(task.PageCookies) > 0 {
		return true
	}
	for name := range task.MediaHeaders {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "authorization", "cookie", "proxy-authorization", "x-api-key", "x-auth-token":
			return true
		}
	}
	for name := range task.PageHeaders {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "authorization", "cookie", "proxy-authorization", "x-api-key", "x-auth-token":
			return true
		}
	}
	return false
}

func persistentHeaderMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for name, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(name))
		switch normalized {
		case "authorization", "cookie", "proxy-authorization", "x-api-key", "x-auth-token":
			continue
		}
		result[name] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func saveBridgeJobs(path string, jobs map[string]*bridgeJob) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	snapshot := persistedBridgeJobs{Version: bridgeJobStoreVersion, Jobs: make([]persistedBridgeJob, 0, len(jobs))}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		snapshot.Jobs = append(snapshot.Jobs, persistedBridgeJob{
			Job: publicBridgeJob(job), Task: persistentBridgeTask(job.task),
			RequiresFreshSession: bridgeTaskHasCredentials(job.task) || job.RequiresFreshSession,
		})
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func loadBridgeJobs(path string) (map[string]*bridgeJob, error) {
	jobs := make(map[string]*bridgeJob)
	if strings.TrimSpace(path) == "" {
		return jobs, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return jobs, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot persistedBridgeJobs
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != bridgeJobStoreVersion {
		return nil, errors.New("unsupported bridge job store version")
	}
	for _, saved := range snapshot.Jobs {
		job := saved.Job
		if strings.TrimSpace(job.ID) == "" {
			continue
		}
		job.task = saved.Task
		job.worker = nil
		job.workerToken = ""
		job.runID = 0
		if job.Status == "running" || job.Status == "queued" || job.Status == "paused" {
			if saved.RequiresFreshSession {
				job.Status = "needs-session"
				job.Detail = "需要从原页面重新提交"
				job.Error = "出于安全考虑，浏览器 Cookie 和授权头不会跨 Bridge 重启保存"
				job.RequiresFreshSession = true
			} else {
				job.Status = "paused"
				job.Detail = "Bridge 已重启，可从检查点继续"
			}
			job.SpeedBytesPerSecond = 0
		}
		jobs[job.ID] = &job
	}
	return jobs, nil
}

func (s *bridgeServer) persistJobsLocked() error {
	return saveBridgeJobs(s.jobStorePath, s.jobs)
}
