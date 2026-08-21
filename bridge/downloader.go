package downkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	downloadAttempts        = 5
	downloadRequestTimeout  = 15 * time.Minute
	directParallelThreshold = 16 << 20
	directTargetPartSize    = 8 << 20
)

var errRangeUnsupported = errors.New("服务器不支持可靠的 HTTP Range")

type goDownloader struct {
	app       *app
	client    *http.Client
	transport *http.Transport
	proxy     string
}

type downloadResult struct {
	segment segment
	bytes   int64
	err     error
}

type downloadHTTPError struct {
	status int
	text   string
}

func (e *downloadHTTPError) Error() string {
	return e.text
}

type remoteFileInfo struct {
	size         int64
	rangeSupport bool
	etag         string
	lastModified string
}

type directDownloadMeta struct {
	URL          string `json:"url"`
	Size         int64  `json:"size"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Parts        int    `json:"parts"`
}

type byteRange struct {
	index int
	start int64
	end   int64
	path  string
}

func newGoDownloader(a *app, proxy string) (*goDownloader, error) {
	transport, err := proxyHTTPTransport(proxy)
	if err != nil {
		return nil, err
	}
	transport.DisableCompression = true
	transport.MaxIdleConns = max(32, a.opts.concurrent*2)
	transport.MaxIdleConnsPerHost = max(16, a.opts.concurrent)
	transport.MaxConnsPerHost = max(16, a.opts.concurrent)
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.DialContext = (&net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	return &goDownloader{
		app:       a,
		client:    a.mediaHTTPClient(transport, 0),
		transport: transport,
		proxy:     proxy,
	}, nil
}

func (d *goDownloader) close() {
	d.transport.CloseIdleConnections()
}

func (d *goDownloader) newRequest(ctx context.Context, sourceURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	d.app.applyTaskMediaRequestHeaders(req)
	return req, nil
}

func (a *app) downloadSegments(all []segment) error {
	missing := missingSegments(all)
	if len(missing) == 0 {
		return a.finishSegments(all)
	}

	proxies := []string{a.opts.proxy}
	var lastErr error
	for pass, proxy := range proxies {
		mode := "直连"
		if proxy != "" {
			mode = "代理 " + proxy
		}
		fmt.Fprintf(consoleOut, "Go 下载器第 %d 轮：%s，并发 %d，待下载 %d 个文件\n", pass+1, mode, a.opts.concurrent, len(missing))

		d, err := newGoDownloader(a, proxy)
		if err != nil {
			return err
		}
		lastErr = d.downloadSegmentBatchWithAll(all, missing)
		d.close()
		missing = missingSegments(all)
		if len(missing) == 0 {
			return a.finishSegments(all)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("下载不完整，仍缺少 %d 个文件；最后错误：%w", len(missing), lastErr)
	}
	return fmt.Errorf("下载不完整，仍缺少 %d 个文件", len(missing))
}

func (a *app) segmentDownloadProgress(percent int) int {
	percent = min(max(percent, 0), 100)
	if a.segmentProgressSpan <= 0 {
		return percent
	}
	return min(a.segmentProgressStart+percent*a.segmentProgressSpan/100, 100)
}

func (a *app) finishSegments(all []segment) error {
	if err := validateHLSKeys(all); err != nil {
		return err
	}
	normalized, err := normalizeTSSegments(all)
	if err != nil {
		return err
	}
	if normalized > 0 {
		fmt.Fprintf(consoleOut, "检测到图片包装的 TS 分片，已自动剥离前缀：%d 个\n", normalized)
	}
	return nil
}

func missingSegments(all []segment) []segment {
	missing := make([]segment, 0)
	for _, s := range all {
		if !segmentFileComplete(s) {
			missing = append(missing, s)
		}
	}
	return missing
}

func segmentFileComplete(s segment) bool {
	info, err := os.Stat(s.path)
	if err != nil || info.Size() == 0 {
		return false
	}
	return s.rangeLength == 0 || info.Size() == s.rangeLength
}

func segmentDownloadedBytes(all []segment) int64 {
	var total int64
	for _, s := range all {
		if segmentFileComplete(s) {
			total += fileSize(s.path)
		} else {
			total += fileSize(s.path + ".part")
		}
	}
	return total
}

func segmentTotalBytes(all []segment) int64 {
	var total int64
	for _, s := range all {
		if s.rangeLength <= 0 {
			return 0
		}
		total += s.rangeLength
	}
	return total
}

func segmentCompletionPercent(all []segment) int {
	if len(all) == 0 {
		return 100
	}
	return (len(all) - len(missingSegments(all))) * 100 / len(all)
}

func (d *goDownloader) downloadSegmentBatch(segments []segment) error {
	return d.downloadSegmentBatchWithAll(segments, segments)
}

func (d *goDownloader) downloadSegmentBatchWithAll(all, segments []segment) error {
	workers := min(max(1, d.app.opts.concurrent), len(segments))
	preexisting := len(all) - len(segments)
	baseBytes := segmentDownloadedBytes(all)
	totalBytes := segmentTotalBytes(all)
	publishJobProgress(d.app.segmentDownloadProgress(preexisting*100/len(all)), fmt.Sprintf("媒体分片 %d/%d · %d 路", preexisting, len(all), workers), baseBytes, totalBytes, 0)
	jobs := make(chan segment)
	results := make(chan downloadResult, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		worker, err := newGoDownloader(d.app, d.proxy)
		if err != nil {
			return err
		}
		group.Add(1)
		go func(worker *goDownloader) {
			defer group.Done()
			defer worker.close()
			for s := range jobs {
				err := worker.downloadSegment(s)
				var downloaded int64
				if err == nil {
					downloaded = fileSize(s.path)
				}
				results <- downloadResult{segment: s, bytes: downloaded, err: err}
			}
		}(worker)
	}
	go func() {
		for _, s := range segments {
			jobs <- s
		}
		close(jobs)
		group.Wait()
		close(results)
	}()

	completed := preexisting
	failed := 0
	startedAt := time.Now()
	var lastErr error
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	report := func() {
		downloadedBytes := segmentDownloadedBytes(all)
		processed := completed + failed
		progress := d.app.segmentDownloadProgress(int(float64(completed) * 100 / float64(len(all))))
		elapsed := time.Since(startedAt).Seconds()
		speedBytesPerSecond := int64(float64(max(downloadedBytes-baseBytes, 0)) / max(elapsed, 0.001))
		publishJobProgress(progress, fmt.Sprintf("媒体分片 %d/%d · %d 路", processed, len(all), workers), downloadedBytes, totalBytes, speedBytesPerSecond)
	}
	for results != nil {
		select {
		case result, ok := <-results:
			if !ok {
				results = nil
				report()
				continue
			}
			if result.err != nil {
				failed++
				lastErr = result.err
				fmt.Fprintf(consoleErr, "文件 %s 下载失败：%v\n", result.segment.name, result.err)
			} else {
				completed++
			}
		case <-ticker.C:
			report()
		}
	}
	return lastErr
}

func (d *goDownloader) downloadSegment(s segment) error {
	if segmentFileComplete(s) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		lastErr = d.downloadSegmentOnce(s)
		if lastErr == nil {
			return nil
		}
		if !retryableDownloadError(lastErr) {
			break
		}
		if attempt < downloadAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}

func (d *goDownloader) downloadSegmentOnce(s segment) error {
	if d.app.useCurlHTTP {
		return d.downloadSegmentWithCurl(s)
	}
	partPath := s.path + ".part"
	partSize := fileSize(partPath)
	if s.rangeLength > 0 && partSize > s.rangeLength {
		_ = os.Remove(partPath)
		partSize = 0
	}
	if s.rangeLength > 0 && partSize == s.rangeLength {
		return replaceFile(partPath, s.path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadRequestTimeout)
	defer cancel()
	req, err := d.newRequest(ctx, s.url)
	if err != nil {
		return err
	}
	requestStart := partSize
	requestEnd := int64(-1)
	if s.rangeLength > 0 {
		requestStart = s.rangeStart + partSize
		requestEnd = s.rangeStart + s.rangeLength - 1
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", requestStart, requestEnd))
	} else if partSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", partSize))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if s.rangeLength == 0 && partSize > 0 && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable) {
		// A normal HLS segment is small, and many CDNs reject Range requests for
		// signed/image-wrapped segment URLs. Discard only the interrupted segment
		// and retry it from byte zero; completed segments remain untouched.
		_ = resp.Body.Close()
		if removeErr := os.Remove(partPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		return d.downloadSegmentOnce(s)
	}
	if resp.StatusCode == http.StatusForbidden && !d.app.useCurlHTTP {
		// Some anti-bot CDNs accept the playlist over Go HTTP but reject segment
		// requests after a worker restart. Retry just this segment through the
		// compatibility transport.
		_ = resp.Body.Close()
		return d.downloadSegmentWithCurl(s)
	}
	appendMode := partSize > 0
	if s.rangeLength > 0 {
		if resp.StatusCode != http.StatusPartialContent {
			return rangeResponseError(resp)
		}
		if err := validateContentRange(resp.Header.Get("Content-Range"), requestStart, requestEnd, -1); err != nil {
			return err
		}
	} else {
		switch {
		case partSize > 0 && resp.StatusCode == http.StatusPartialContent:
			if err := validateContentRange(resp.Header.Get("Content-Range"), partSize, -1, -1); err != nil {
				return err
			}
		case partSize > 0 && resp.StatusCode >= 200 && resp.StatusCode < 300:
			appendMode = false
			partSize = 0
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
		default:
			return responseDownloadError(resp)
		}
	}

	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return fmt.Errorf("响应长度不完整：预期 %d，实际 %d", resp.ContentLength, written)
	}
	if s.rangeLength > 0 && fileSize(partPath) != s.rangeLength {
		return fmt.Errorf("Range 文件长度错误：预期 %d，实际 %d", s.rangeLength, fileSize(partPath))
	}
	return replaceFile(partPath, s.path)
}

func (a *app) downloadDirectFile(sourceURL, outputPath string) error {
	proxies := []string{a.opts.proxy}
	var lastErr error
	for _, proxy := range proxies {
		mode := "直连"
		if proxy != "" {
			mode = "代理 " + proxy
		}
		fmt.Fprintf(consoleOut, "探测直链：%s\n", mode)
		d, err := newGoDownloader(a, proxy)
		if err != nil {
			return err
		}
		info, err := d.probeFile(sourceURL)
		if err == nil {
			recovered := directCheckpointBytes(outputPath)
			publishJobPhaseProgress("resolving", 100, "解析完成", 0, 0, 0)
			progress := 0
			if info.size > 0 {
				progress = int(min(recovered, info.size) * 100 / info.size)
			}
			detail := "准备下载 MP4"
			if recovered > 0 {
				detail = "正在从 MP4 续传点恢复"
			}
			publishJobPhaseProgress("downloading", progress, detail, recovered, info.size, 0)
			if info.rangeSupport && info.size >= directParallelThreshold {
				parts := directPartCount(info.size, a.opts.concurrent)
				fmt.Fprintf(consoleOut, "服务器支持 Range：%.2f MiB，使用 %d 段并行下载\n", float64(info.size)/(1<<20), parts)
				err = d.downloadRangedFile(sourceURL, outputPath, info, parts)
				if errors.Is(err, errRangeUnsupported) {
					d.cleanupRangedFiles(outputPath, parts)
					fmt.Fprintln(consoleOut, "服务器未稳定支持 Range，退回单连接下载。")
					err = d.downloadStreamFile(sourceURL, outputPath, info)
				}
			} else {
				if info.size > 0 {
					fmt.Fprintf(consoleOut, "使用单连接下载：%.2f MiB\n", float64(info.size)/(1<<20))
				} else {
					fmt.Fprintln(consoleOut, "文件长度未知，使用单连接下载。")
				}
				err = d.downloadStreamFile(sourceURL, outputPath, info)
			}
		}
		d.close()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("MP4 下载失败：%w", lastErr)
}

func directCheckpointBytes(outputPath string) int64 {
	if size := fileSize(outputPath + ".part"); size > 0 {
		return size
	}
	data, err := os.ReadFile(directMetaPath(outputPath))
	if err != nil {
		return 0
	}
	var meta directDownloadMeta
	if json.Unmarshal(data, &meta) != nil || meta.Parts <= 0 {
		return 0
	}
	var total int64
	for index := 0; index < meta.Parts; index++ {
		path := directPartPath(outputPath, index)
		size := fileSize(path)
		if size == 0 {
			size = fileSize(path + ".part")
		}
		total += size
	}
	return total
}

func (d *goDownloader) probeFile(sourceURL string) (remoteFileInfo, error) {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		req, err := d.newRequest(ctx, sourceURL)
		if err != nil {
			cancel()
			return remoteFileInfo{}, err
		}
		req.Header.Set("Range", "bytes=0-0")
		resp, err := d.client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			info, probeErr := inspectProbeResponse(resp)
			_ = resp.Body.Close()
			cancel()
			if probeErr == nil {
				return info, nil
			}
			lastErr = probeErr
		}
		if !retryableDownloadError(lastErr) {
			break
		}
		if attempt < downloadAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return remoteFileInfo{}, lastErr
}

func inspectProbeResponse(resp *http.Response) (remoteFileInfo, error) {
	info := remoteFileInfo{
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		start, end, total, totalKnown, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil || start != 0 || end != 0 || !totalKnown || total <= 0 {
			return remoteFileInfo{}, fmt.Errorf("Range 探测响应无效：%s", resp.Header.Get("Content-Range"))
		}
		info.size = total
		info.rangeSupport = true
		return info, nil
	case http.StatusOK:
		info.size = resp.ContentLength
		return info, nil
	default:
		return remoteFileInfo{}, responseDownloadError(resp)
	}
}

func directPartCount(size int64, configured int) int {
	parts := max(configured, 1)
	bySize := int((size + directTargetPartSize - 1) / directTargetPartSize)
	parts = min(parts, max(1, bySize))
	return parts
}

func (d *goDownloader) downloadRangedFile(sourceURL, outputPath string, info remoteFileInfo, parts int) error {
	meta := directDownloadMeta{
		URL:          sourceURL,
		Size:         info.size,
		ETag:         info.etag,
		LastModified: info.lastModified,
		Parts:        parts,
	}
	if err := prepareDirectMeta(outputPath, meta); err != nil {
		return err
	}
	ranges := makeDirectRanges(outputPath, info.size, parts)
	workers := min(parts, max(1, d.app.opts.concurrent))
	stopProgress := startByteProgress(fmt.Sprintf("MP4（%d 路）", workers), info.size, func() int64 {
		var downloaded int64
		for _, part := range ranges {
			expected := part.end - part.start + 1
			size := fileSize(part.path)
			if size == 0 {
				size = fileSize(part.path + ".part")
			}
			downloaded += min(size, expected)
		}
		return downloaded
	})
	defer stopProgress()
	jobs := make(chan byteRange)
	results := make(chan error, parts)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		worker, err := newGoDownloader(d.app, d.proxy)
		if err != nil {
			return err
		}
		group.Add(1)
		go func(worker *goDownloader) {
			defer group.Done()
			defer worker.close()
			for part := range jobs {
				if fileSize(part.path) == part.end-part.start+1 {
					results <- nil
					continue
				}
				results <- worker.downloadRangePart(sourceURL, part, info.size)
			}
		}(worker)
	}
	go func() {
		for _, part := range ranges {
			jobs <- part
		}
		close(jobs)
		group.Wait()
		close(results)
	}()

	var lastErr error
	for err := range results {
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return mergeDirectParts(outputPath, ranges, info.size)
}

func (d *goDownloader) downloadRangePart(sourceURL string, part byteRange, total int64) error {
	tempPath := part.path + ".part"
	expected := part.end - part.start + 1
	if size := fileSize(tempPath); size > expected {
		_ = os.Remove(tempPath)
	}
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		offset := fileSize(tempPath)
		if offset == expected {
			return replaceFile(tempPath, part.path)
		}
		ctx, cancel := context.WithTimeout(context.Background(), downloadRequestTimeout)
		req, err := d.newRequest(ctx, sourceURL)
		if err == nil {
			start := part.start + offset
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, part.end))
			resp, requestErr := d.client.Do(req)
			if requestErr != nil {
				err = requestErr
			} else {
				err = d.appendRangeResponse(resp, tempPath, start, part.end, total)
			}
		}
		cancel()
		if err == nil && fileSize(tempPath) == expected {
			return replaceFile(tempPath, part.path)
		}
		lastErr = err
		if errors.Is(err, errRangeUnsupported) || !retryableDownloadError(err) {
			break
		}
		if attempt < downloadAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}

func (d *goDownloader) appendRangeResponse(resp *http.Response, path string, start, end, total int64) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusOK {
			return errRangeUnsupported
		}
		return responseDownloadError(resp)
	}
	if err := validateContentRange(resp.Header.Get("Content-Range"), start, end, total); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != end-start+1 {
		return fmt.Errorf("Range 响应长度错误：预期 %d，实际 %d", end-start+1, written)
	}
	return nil
}

func (d *goDownloader) downloadStreamFile(sourceURL, outputPath string, info remoteFileInfo) error {
	partPath := outputPath + ".part"
	if info.size > 0 && fileSize(partPath) > info.size {
		_ = os.Remove(partPath)
	}
	stopProgress := startByteProgress("MP4（单连接）", info.size, func() int64 {
		if size := fileSize(partPath); size > 0 {
			return size
		}
		return fileSize(outputPath)
	})
	defer stopProgress()
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		offset := fileSize(partPath)
		if info.size > 0 && offset == info.size {
			return replaceFile(partPath, outputPath)
		}
		ctx, cancel := context.WithTimeout(context.Background(), downloadRequestTimeout)
		req, err := d.newRequest(ctx, sourceURL)
		if err == nil && offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		if err == nil {
			resp, requestErr := d.client.Do(req)
			if requestErr != nil {
				err = requestErr
			} else {
				err = writeStreamResponse(resp, partPath, offset, info.size)
			}
		}
		cancel()
		if err == nil {
			if info.size <= 0 || fileSize(partPath) == info.size {
				return replaceFile(partPath, outputPath)
			}
			err = fmt.Errorf("文件长度错误：预期 %d，实际 %d", info.size, fileSize(partPath))
		}
		lastErr = err
		if !retryableDownloadError(err) {
			break
		}
		if attempt < downloadAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return lastErr
}

func startByteProgress(label string, total int64, current func() int64) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	initial := current()
	startedAt := time.Now()
	report := func() {
		now := time.Now()
		downloaded := current()
		elapsed := now.Sub(startedAt).Seconds()
		newBytes := max(downloaded-initial, int64(0))
		speedBytesPerSecond := int64(float64(newBytes) / max(elapsed, 0.001))
		speed := float64(speedBytesPerSecond) / (1024 * 1024)
		if total > 0 {
			percent := min(float64(downloaded)*100/float64(total), 100)
			publishJobProgress(int(percent), label+" 下载中", downloaded, total, speedBytesPerSecond)
			remaining := ""
			if speed > 0 && downloaded < total {
				seconds := float64(total-downloaded) / (speed * 1024 * 1024)
				remaining = fmt.Sprintf("，预计剩余 %s", (time.Duration(seconds) * time.Second).Round(time.Second))
			}
			fmt.Fprintf(consoleOut, "%s 下载进度：%.1f%%（%.2f/%.2f MiB），平均 %.2f MiB/s%s\n",
				label, percent, float64(downloaded)/(1<<20), float64(total)/(1<<20), speed, remaining)
			return
		}
		publishJobProgress(0, label+" 下载中", downloaded, 0, speedBytesPerSecond)
		fmt.Fprintf(consoleOut, "%s 下载进度：%.2f MiB，平均 %.2f MiB/s\n", label, float64(downloaded)/(1<<20), speed)
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				report()
			case <-stop:
				report()
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func writeStreamResponse(resp *http.Response, path string, offset, total int64) error {
	defer resp.Body.Close()
	appendMode := offset > 0 && resp.StatusCode == http.StatusPartialContent
	if appendMode {
		if err := validateContentRange(resp.Header.Get("Content-Range"), offset, -1, total); err != nil {
			return err
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseDownloadError(resp)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return fmt.Errorf("响应长度不完整：预期 %d，实际 %d", resp.ContentLength, written)
	}
	return nil
}

func prepareDirectMeta(outputPath string, meta directDownloadMeta) error {
	metaPath := directMetaPath(outputPath)
	var old directDownloadMeta
	oldData, oldErr := os.ReadFile(metaPath)
	if oldErr == nil && json.Unmarshal(oldData, &old) == nil && old == meta {
		return nil
	}
	if old.Parts > 0 {
		removeDirectParts(outputPath, old.Parts)
	}
	removeDirectParts(outputPath, meta.Parts)
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0o644)
}

func makeDirectRanges(outputPath string, size int64, parts int) []byteRange {
	ranges := make([]byteRange, 0, parts)
	for index := 0; index < parts; index++ {
		start := size * int64(index) / int64(parts)
		end := size*int64(index+1)/int64(parts) - 1
		ranges = append(ranges, byteRange{
			index: index,
			start: start,
			end:   end,
			path:  directPartPath(outputPath, index),
		})
	}
	return ranges
}

func mergeDirectParts(outputPath string, ranges []byteRange, expected int64) error {
	partial := outputPath + ".part"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, part := range ranges {
		input, openErr := os.Open(part.path)
		if openErr != nil {
			_ = file.Close()
			return openErr
		}
		_, copyErr := io.Copy(file, input)
		_ = input.Close()
		if copyErr != nil {
			_ = file.Close()
			return copyErr
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	if fileSize(partial) != expected {
		return fmt.Errorf("合并后文件长度错误：预期 %d，实际 %d", expected, fileSize(partial))
	}
	if err := replaceFile(partial, outputPath); err != nil {
		return err
	}
	removeDirectParts(outputPath, len(ranges))
	_ = os.Remove(directMetaPath(outputPath))
	return nil
}

func (d *goDownloader) cleanupRangedFiles(outputPath string, parts int) {
	removeDirectParts(outputPath, parts)
	_ = os.Remove(directMetaPath(outputPath))
}

func removeDirectParts(outputPath string, parts int) {
	for index := 0; index < parts; index++ {
		path := directPartPath(outputPath, index)
		_ = os.Remove(path)
		_ = os.Remove(path + ".part")
	}
}

func directMetaPath(outputPath string) string {
	return outputPath + ".downkit.json"
}

func directPartPath(outputPath string, index int) string {
	return fmt.Sprintf("%s.downkit.part%02d", outputPath, index)
}

func parseContentRange(value string) (start, end, total int64, totalKnown bool, err error) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bytes") {
		return 0, 0, 0, false, errors.New("缺少有效 Content-Range")
	}
	pieces := strings.SplitN(fields[1], "/", 2)
	span := strings.SplitN(pieces[0], "-", 2)
	if len(pieces) != 2 || len(span) != 2 {
		return 0, 0, 0, false, errors.New("Content-Range 格式无效")
	}
	start, err = strconv.ParseInt(span[0], 10, 64)
	if err != nil {
		return 0, 0, 0, false, err
	}
	end, err = strconv.ParseInt(span[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, 0, false, errors.New("Content-Range 范围无效")
	}
	if pieces[1] != "*" {
		total, err = strconv.ParseInt(pieces[1], 10, 64)
		if err != nil || total <= end {
			return 0, 0, 0, false, errors.New("Content-Range 总长度无效")
		}
		totalKnown = true
	}
	return start, end, total, totalKnown, nil
}

func validateContentRange(value string, wantStart, wantEnd, wantTotal int64) error {
	start, end, total, totalKnown, err := parseContentRange(value)
	if err != nil {
		return err
	}
	if start != wantStart {
		return fmt.Errorf("Content-Range 起点错误：预期 %d，实际 %d", wantStart, start)
	}
	if wantEnd >= 0 && end != wantEnd {
		return fmt.Errorf("Content-Range 终点错误：预期 %d，实际 %d", wantEnd, end)
	}
	if wantTotal > 0 && (!totalKnown || total != wantTotal) {
		return fmt.Errorf("Content-Range 总长度错误：预期 %d，实际 %d", wantTotal, total)
	}
	return nil
}

func rangeResponseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return errRangeUnsupported
	}
	return responseDownloadError(resp)
}

func responseDownloadError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := fmt.Sprintf("HTTP %s", resp.Status)
	if preview := responsePreview(body); preview != "空响应" {
		message += "：" + preview
	}
	return &downloadHTTPError{status: resp.StatusCode, text: message}
}

func retryableDownloadError(err error) bool {
	if err == nil || errors.Is(err, errRangeUnsupported) {
		return false
	}
	var httpErr *downloadHTTPError
	if errors.As(err, &httpErr) {
		return retryableHTTPStatus(httpErr.status)
	}
	return true
}

func replaceFile(source, destination string) error {
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, destination)
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
