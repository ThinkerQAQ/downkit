package downkit

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The console serves the same extension UI shipped with DownKit.
// Privileged API calls MUST go through the browser extension relay; the HTTP
// server never exposes the Native Messaging token or privileged endpoints.
func extensionAssetsDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	binaryDir := filepath.Dir(executable)
	workspace, _ := os.Getwd()
	candidates := []string{
		filepath.Join(binaryDir, "extension"),
		filepath.Join(binaryDir, "..", "extension"),
		filepath.Join(binaryDir, "..", "..", "extension"),
		filepath.Join(workspace, "extension"),
		filepath.Join(workspace, "..", "extension"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "popup", "popup.html")); err == nil && info.Mode().IsRegular() {
			return filepath.Clean(candidate), nil
		}
	}
	return "", os.ErrNotExist
}

func consoleHostAllowed(host string) bool {
	name, port, err := net.SplitHostPort(host)
	return err == nil && port == "17891" && (name == "127.0.0.1" || name == "localhost")
}

func (s *bridgeServer) handleConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !consoleHostAllowed(r.Host) {
		http.Error(w, "invalid console host", http.StatusForbidden)
		return
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") &&
		!strings.EqualFold(r.Header.Get("Sec-Fetch-Mode"), "navigate") {
		http.Error(w, "cross-site console resources forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; "+
			"connect-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	if r.URL.Path == "/console" || r.URL.Path == "/console/" {
		http.Redirect(w, r, "/console/popup/popup.html", http.StatusFound)
		return
	}
	directory, err := extensionAssetsDir()
	if err != nil {
		http.Error(w, "extension assets not installed next to Bridge", http.StatusServiceUnavailable)
		return
	}
	http.StripPrefix("/console/", http.FileServer(http.Dir(directory))).ServeHTTP(w, r)
}
