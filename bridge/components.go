package downkit

import (
	"bufio"
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

const ytDLPReleaseBaseURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"

func ytDLPReleaseAsset(goos, goarch string) (string, string, error) {
	switch goos {
	case "windows":
		if goarch == "arm64" {
			return "yt-dlp_arm64.exe", "yt-dlp.exe", nil
		}
		if goarch == "amd64" {
			return "yt-dlp.exe", "yt-dlp.exe", nil
		}
	case "linux":
		if goarch == "arm64" {
			return "yt-dlp_linux_aarch64", "yt-dlp", nil
		}
		if goarch == "amd64" {
			return "yt-dlp_linux", "yt-dlp", nil
		}
	case "darwin":
		if goarch == "amd64" || goarch == "arm64" {
			return "yt-dlp_macos", "yt-dlp", nil
		}
	}
	return "", "", fmt.Errorf("yt-dlp 暂不支持自动安装到 %s/%s", goos, goarch)
}

func componentHTTPClient(proxy string) (*http.Client, error) {
	transport, err := proxyHTTPTransport(proxy)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Minute}, nil
}

func fetchLimited(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "DownKit/"+bridgeVersion)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 %s 失败：HTTP %d", filepath.Base(request.URL.Path), response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, errors.New("组件文件超过大小限制")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("组件文件超过大小限制")
	}
	return data, nil
}

func checksumForAsset(checksums []byte, asset string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(checksums)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == asset {
			sum := strings.ToLower(fields[0])
			if len(sum) != sha256.Size*2 {
				break
			}
			if _, err := hex.DecodeString(sum); err == nil {
				return sum, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("官方校验文件中没有 %s", asset)
}

func downloadVerifiedComponent(ctx context.Context, client *http.Client, baseURL, asset, target string) error {
	checksums, err := fetchLimited(ctx, client, strings.TrimRight(baseURL, "/")+"/SHA2-256SUMS", 1<<20)
	if err != nil {
		return fmt.Errorf("无法下载组件校验文件：%w", err)
	}
	want, err := checksumForAsset(checksums, asset)
	if err != nil {
		return err
	}
	binary, err := fetchLimited(ctx, client, strings.TrimRight(baseURL, "/")+"/"+asset, 128<<20)
	if err != nil {
		return fmt.Errorf("无法下载组件：%w", err)
	}
	gotBytes := sha256.Sum256(binary)
	got := hex.EncodeToString(gotBytes[:])
	if got != want {
		return fmt.Errorf("组件 SHA-256 校验失败：期望 %s，实际 %s", want, got)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary := target + ".part"
	if err := os.WriteFile(temporary, binary, 0o700); err != nil {
		return err
	}
	if err := replaceFile(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func installYTDLPComponent(ctx context.Context, proxy string) (string, error) {
	asset, installedName, err := ytDLPReleaseAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	toolsDir, err := bridgeDataPath("tools")
	if err != nil {
		return "", err
	}
	target := filepath.Join(toolsDir, installedName)
	client, err := componentHTTPClient(proxy)
	if err != nil {
		return "", err
	}
	if err := downloadVerifiedComponent(ctx, client, ytDLPReleaseBaseURL, asset, target); err != nil {
		return "", err
	}
	return target, nil
}
