package downkit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

var workerProgress = struct {
	sync.RWMutex
	jobID string
}{}

func beginJobReporting(jobID string) {
	workerProgress.Lock()
	workerProgress.jobID = jobID
	workerProgress.Unlock()
}

func publishJobProgress(progress int, detail string, downloaded, total, speedBytesPerSecond int64) {
	publishJobPhaseProgress("downloading", progress, detail, downloaded, total, speedBytesPerSecond)
}

func publishJobPhaseProgress(phase string, progress int, detail string, downloaded, total, speedBytesPerSecond int64) {
	workerProgress.RLock()
	jobID := workerProgress.jobID
	workerProgress.RUnlock()
	if jobID == "" {
		return
	}
	reportJobProgress(jobID, phase, progress, detail, downloaded, total, speedBytesPerSecond)
}

func reportJobProgress(jobID, phase string, progress int, detail string, downloaded, total, speedBytesPerSecond int64) {
	state, err := readBridgeState()
	if err != nil {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"phase": phase, "progress": progress, "detail": detail,
		"downloadedBytes": downloaded, "totalBytes": total,
		"speedBytesPerSecond": speedBytesPerSecond,
	})
	request, _ := http.NewRequest(http.MethodPost, state.BaseURL+"/v1/jobs/"+jobID+"/progress", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-DownKit-Token", state.Token)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func publishJobOutput(path string) {
	publishJobOutputFile(path, 0, "")
}

func publishJobFiles(files []bridgeJobFile) {
	workerProgress.RLock()
	jobID := workerProgress.jobID
	workerProgress.RUnlock()
	if jobID == "" || len(files) == 0 {
		return
	}
	state, err := readBridgeState()
	if err != nil {
		return
	}
	body, _ := json.Marshal(map[string]any{"files": files})
	request, _ := http.NewRequest(http.MethodPost, state.BaseURL+"/v1/jobs/"+jobID+"/files", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-DownKit-Token", state.Token)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func publishJobOutputFile(path string, index int, itemID string) {
	workerProgress.RLock()
	jobID := workerProgress.jobID
	workerProgress.RUnlock()
	if jobID == "" || path == "" {
		return
	}
	state, err := readBridgeState()
	if err != nil {
		return
	}
	body, _ := json.Marshal(map[string]any{"path": path, "index": index, "id": itemID})
	request, _ := http.NewRequest(http.MethodPost, state.BaseURL+"/v1/jobs/"+jobID+"/output", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-DownKit-Token", state.Token)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}
