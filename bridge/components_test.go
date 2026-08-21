package downkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestYTDLPReleaseAsset(t *testing.T) {
	tests := []struct {
		goos, goarch, asset, installed string
	}{
		{"windows", "amd64", "yt-dlp.exe", "yt-dlp.exe"},
		{"windows", "arm64", "yt-dlp_arm64.exe", "yt-dlp.exe"},
		{"linux", "amd64", "yt-dlp_linux", "yt-dlp"},
		{"linux", "arm64", "yt-dlp_linux_aarch64", "yt-dlp"},
		{"darwin", "arm64", "yt-dlp_macos", "yt-dlp"},
	}
	for _, test := range tests {
		asset, installed, err := ytDLPReleaseAsset(test.goos, test.goarch)
		if err != nil || asset != test.asset || installed != test.installed {
			t.Fatalf("ytDLPReleaseAsset(%s, %s) = %q, %q, %v", test.goos, test.goarch, asset, installed, err)
		}
	}
	if _, _, err := ytDLPReleaseAsset("android", "arm64"); err == nil {
		t.Fatal("expected Android automatic install rejection")
	}
}

func TestDownloadVerifiedComponent(t *testing.T) {
	binary := []byte("verified component")
	digest := sha256.Sum256(binary)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/SHA2-256SUMS":
			_, _ = fmt.Fprintf(response, "%s  component.bin\n", hex.EncodeToString(digest[:]))
		case "/component.bin":
			_, _ = response.Write(binary)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "tools", "component")
	if err := downloadVerifiedComponent(context.Background(), server.Client(), server.URL, "component.bin", target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != string(binary) {
		t.Fatalf("unexpected component: %q, %v", data, err)
	}
}

func TestDownloadVerifiedComponentRejectsHashMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/SHA2-256SUMS" {
			_, _ = fmt.Fprintln(response, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  component.bin")
			return
		}
		_, _ = response.Write([]byte("tampered"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "component")
	if err := downloadVerifiedComponent(context.Background(), server.Client(), server.URL, "component.bin", target); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("unverified component was written: %v", err)
	}
}
