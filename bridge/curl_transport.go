package downkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const curlRequestTimeout = 15 * time.Minute

func curlExecutable() (string, error) {
	names := []string{"curl"}
	if runtime.GOOS == "windows" {
		names = []string{"curl.exe", "curl"}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("未找到 curl，无法启用 HTTP 兼容模式")
}

func (a *app) curlBaseArgs(sourceURL, outputPath string, updateCookies bool) ([]string, error) {
	request, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	applyMediaRequestHeaders(request, a.opts)

	args := []string{
		"--silent", "--show-error",
		"--connect-timeout", "20",
		"--max-time", strconv.Itoa(int(curlRequestTimeout.Seconds())),
		"--proto", "=http,https",
		"--output", outputPath,
		"--write-out", "%{http_code}",
	}
	if a.opts.proxy != "" {
		args = append(args, "--proxy", a.opts.proxy)
	} else {
		// Respect DownKit's explicit direct mode even if curl inherits proxy
		// environment variables from the desktop session.
		args = append(args, "--noproxy", "*")
	}

	names := make([]string, 0, len(request.Header))
	for name := range request.Header {
		if !strings.EqualFold(name, "Cookie") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range request.Header.Values(name) {
			args = append(args, "--header", name+": "+value)
		}
	}

	cookieFile := filepath.Join(a.workDir, "curl-cookies.txt")
	if _, err := os.Stat(cookieFile); err == nil {
		args = append(args, "--cookie", cookieFile)
	} else if len(a.opts.mediaCookies) > 0 {
		generated, writeErr := writeTaskCookieFile(a.workDir, "curl-cookies.txt", a.opts.mediaCookies)
		if writeErr != nil {
			return nil, writeErr
		}
		args = append(args, "--cookie", generated)
	}
	if updateCookies {
		args = append(args, "--cookie-jar", cookieFile)
	}
	args = append(args, "--url", sourceURL)
	return args, nil
}

func (a *app) runCurlRequest(ctx context.Context, sourceURL, outputPath, byteRange string, updateCookies bool) (int, error) {
	path, err := curlExecutable()
	if err != nil {
		return 0, err
	}
	args, err := a.curlBaseArgs(sourceURL, outputPath, updateCookies)
	if err != nil {
		return 0, err
	}
	if byteRange != "" {
		// Insert before --url so a URL-like value can never be parsed as an
		// additional transfer target.
		tail := append([]string(nil), args[len(args)-2:]...)
		args = append(args[:len(args)-2], "--range", byteRange)
		args = append(args, tail...)
	}
	command := exec.CommandContext(ctx, path, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	status, parseErr := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = runErr.Error()
		}
		return status, fmt.Errorf("curl 请求失败：%s", detail)
	}
	if parseErr != nil {
		return 0, fmt.Errorf("curl 未返回有效 HTTP 状态：%q", stdout.String())
	}
	return status, nil
}

func (a *app) fetchHTTPCurl(sourceURL string) ([]byte, int, error) {
	if err := os.MkdirAll(a.workDir, 0o755); err != nil {
		return nil, 0, err
	}
	file, err := os.CreateTemp(a.workDir, ".curl-playlist-*.tmp")
	if err != nil {
		return nil, 0, err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return nil, 0, err
	}
	defer os.Remove(path)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	status, requestErr := a.runCurlRequest(ctx, sourceURL, path, "", true)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, status, readErr
	}
	if len(data) > 16<<20 {
		return nil, status, errors.New("响应超过 16 MiB 限制")
	}
	if requestErr != nil {
		return nil, status, requestErr
	}
	if status < 200 || status >= 300 {
		return nil, status, fmt.Errorf("HTTP %d：%s", status, responsePreview(data))
	}
	return data, status, nil
}

func (d *goDownloader) downloadSegmentWithCurl(s segment) error {
	partPath := s.path + ".part"
	if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	byteRange := ""
	if s.rangeLength > 0 {
		byteRange = fmt.Sprintf("%d-%d", s.rangeStart, s.rangeStart+s.rangeLength-1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadRequestTimeout)
	defer cancel()
	status, err := d.app.runCurlRequest(ctx, s.url, partPath, byteRange, false)
	if err != nil {
		return err
	}
	if s.rangeLength > 0 && status != http.StatusPartialContent {
		return fmt.Errorf("Range 请求返回 HTTP %d，预期 206", status)
	}
	if s.rangeLength == 0 && (status < 200 || status >= 300) {
		return fmt.Errorf("HTTP %d", status)
	}
	info, err := os.Stat(partPath)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return io.ErrUnexpectedEOF
	}
	if s.rangeLength > 0 && info.Size() != s.rangeLength {
		return fmt.Errorf("Range 文件长度错误：预期 %d，实际 %d", s.rangeLength, info.Size())
	}
	return replaceFile(partPath, s.path)
}
