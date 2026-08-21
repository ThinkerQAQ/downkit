package downkit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePlaylistProbeReturnsSingleForDirectMedia(t *testing.T) {
	server := &bridgeServer{
		state:  bridgeState{Token: "secret"},
		config: defaultBridgeConfig(),
	}
	body, err := json.Marshal(bridgeTask{
		URL:      "https://media.test/video.mp4",
		Title:    "video",
		Playlist: "single",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/playlist/probe", bytes.NewReader(body))
	request.Header.Set("X-DownKit-Token", "secret")
	response := httptest.NewRecorder()

	server.handlePlaylistProbe(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	var result struct {
		OK       bool `json:"ok"`
		Playlist bool `json:"playlist"`
		Count    int  `json:"count"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Playlist || result.Count != 1 {
		t.Fatalf("unexpected probe result: %+v", result)
	}
}

func TestHandlePlaylistProbeRejectsUnauthorizedRequest(t *testing.T) {
	server := &bridgeServer{state: bridgeState{Token: "secret"}}
	request := httptest.NewRequest(http.MethodPost, "/v1/playlist/probe", bytes.NewReader([]byte(`{}`)))
	response := httptest.NewRecorder()
	server.handlePlaylistProbe(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want %d", response.Code, http.StatusUnauthorized)
	}
}
