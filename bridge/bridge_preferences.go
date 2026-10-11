package downkit

import (
	"net/http"
)

type preferenceUpdate struct {
	OpenMode   *string `json:"openMode,omitempty"`
	UILanguage *string `json:"uiLanguage,omitempty"`
	ShowLogs   *bool   `json:"showLogs,omitempty"`
	DetectHls  *bool   `json:"detectHls,omitempty"`
	DetectDash *bool   `json:"detectDash,omitempty"`
	DetectMp4  *bool   `json:"detectMp4,omitempty"`
}

func preferencesOf(c bridgeConfig) map[string]any {
	return map[string]any{
		"openMode": c.OpenMode, "uiLanguage": c.UILanguage, "showLogs": c.ShowLogs,
		"detectHls": c.DetectHls, "detectDash": c.DetectDash, "detectMp4": c.DetectMp4,
	}
}
func (s *bridgeServer) handlePreferences(w http.ResponseWriter, r *http.Request) {
	s.allowExtension(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preferences": preferencesOf(s.config)})
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var update preferenceUpdate
	if decodeBridgeJSON(w, r, &update) != nil {
		return
	}
	c := s.config
	if update.OpenMode != nil {
		c.OpenMode = *update.OpenMode
	}
	if update.UILanguage != nil {
		c.UILanguage = *update.UILanguage
	}
	if update.ShowLogs != nil {
		c.ShowLogs = *update.ShowLogs
	}
	if update.DetectHls != nil {
		c.DetectHls = *update.DetectHls
	}
	if update.DetectDash != nil {
		c.DetectDash = *update.DetectDash
	}
	if update.DetectMp4 != nil {
		c.DetectMp4 = *update.DetectMp4
	}
	c, err := normalizeBridgeConfig(c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err = saveBridgeConfig(c); err != nil {
		appendRuntimeEvent("ERROR", "bridge", "preferences_save_failed", "reason", "io")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot save TOML"})
		return
	}
	s.config = c
	appendRuntimeEvent("INFO", "bridge", "preferences_saved")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "preferences": preferencesOf(c)})
}
