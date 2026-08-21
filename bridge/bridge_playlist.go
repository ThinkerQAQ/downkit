package downkit

import (
	"net/http"
	"os"
	"strings"
)

type playlistProbeResult struct {
	Playlist bool   `json:"playlist"`
	Count    int    `json:"count"`
	Title    string `json:"title,omitempty"`
}

func (s *bridgeServer) handlePlaylistProbe(response http.ResponseWriter, request *http.Request) {
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

	result, err := probeBridgePlaylist(task, config)
	if err != nil {
		writeJSON(response, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"ok":       true,
		"playlist": result.Playlist,
		"count":    result.Count,
		"title":    result.Title,
	})
}

func probeBridgePlaylist(task bridgeTask, config bridgeConfig) (playlistProbeResult, error) {
	opts, err := optionsFromBridgeTask(task, config)
	if err != nil {
		return playlistProbeResult{}, err
	}
	pageURL, resolvePage := pageForSeparatedMedia(opts.sourceURL, opts.referer, opts.resolvePage)
	if !resolvePage {
		return playlistProbeResult{Count: 1}, nil
	}

	opts.ytDLPPath, err = findTool(opts.ytDLPPath, "yt-dlp")
	if err != nil {
		return playlistProbeResult{}, err
	}
	workDir, err := os.MkdirTemp("", "downkit-playlist-probe-")
	if err != nil {
		return playlistProbeResult{}, err
	}
	defer os.RemoveAll(workDir)

	a := &app{opts: opts, workDir: workDir}
	access, err := a.prepareYTDLPAccess(pageURL)
	if err != nil {
		return playlistProbeResult{}, err
	}
	defer access.cleanup()

	info, _, err := a.probeYTDLPPlaylist(pageURL, access.attempts)
	if err != nil {
		return playlistProbeResult{}, err
	}
	count := info.count()
	return playlistProbeResult{
		Playlist: strings.EqualFold(info.Type, "playlist") && count > 1,
		Count:    count,
		Title:    strings.TrimSpace(info.Title),
	}, nil
}
