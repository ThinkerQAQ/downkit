package downkit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36 Edg/151.0.0.0"

var (
	bandwidthRE           = regexp.MustCompile(`(?:^|[:,])BANDWIDTH=(\d+)`)
	heightRE              = regexp.MustCompile(`(?:^|[:,])RESOLUTION=\d+x(\d+)`)
	mapURIRE              = regexp.MustCompile(`(?i)URI="([^"]+)"`)
	methodRE              = regexp.MustCompile(`(?i)(?:^|[:,])METHOD=([^,]+)`)
	keyFormatRE           = regexp.MustCompile(`(?i)(?:^|[:,])KEYFORMAT="?([^",]+)`)
	ffmpegStableVersionRE = regexp.MustCompile(`(?i)^ffmpeg version\s+n?(\d+)\.(\d+)`)
	ffmpegDatedVersionRE  = regexp.MustCompile(`(?i)^ffmpeg version\s+(\d{4})-(\d{2})-(\d{2})-git`)
	ffmpegNVersionRE      = regexp.MustCompile(`(?i)^ffmpeg version\s+n-(\d+)-g[0-9a-f]+`)
	ytDLPExtractorErrorRE = regexp.MustCompile(`(?im)^ERROR:\s*\[([^\]]+)\]`)
)

type options struct {
	sourceURL          string
	title              string
	referer            string
	origin             string
	userAgent          string
	proxy              string
	outputDir          string
	ffmpegPath         string
	ytDLPPath          string
	playlistMode       string
	quality            int
	qualitySet         bool
	limit              int
	concurrent         int
	keepWork           bool
	requestHeaders     map[string]string
	pageRequestHeaders map[string]string
	mediaCookies       []browserCookie
	pageCookies        []browserCookie
	resolvePage        bool
	jobID              string
}

// browserCookie is the structured, task-scoped representation sent by the
// extension. Cookie values never travel in request headers, CLI flags or URLs.
type browserCookie struct {
	Name           string         `json:"name"`
	Value          string         `json:"value"`
	Domain         string         `json:"domain"`
	HostOnly       bool           `json:"hostOnly"`
	Path           string         `json:"path"`
	Secure         bool           `json:"secure"`
	HTTPOnly       bool           `json:"httpOnly"`
	SameSite       string         `json:"sameSite"`
	Session        bool           `json:"session"`
	ExpirationDate float64        `json:"expirationDate,omitempty"`
	PartitionKey   map[string]any `json:"partitionKey,omitempty"`
}

type variant struct {
	url        string
	bandwidth  int64
	height     int
	audioGroup string
}

type mediaRendition struct {
	url        string
	name       string
	language   string
	isDefault  bool
	autoSelect bool
}

type segment struct {
	url         string
	name        string
	path        string
	rangeStart  int64
	rangeLength int64
	encrypted   bool
}

type app struct {
	opts                 options
	workDir              string
	segmentDir           string
	localPlaylist        string
	muxer                mediaMuxer
	mediaSession         *mediaHTTPSession
	useCurlHTTP          bool
	segmentProgressStart int
	segmentProgressSpan  int
}

var (
	consoleOut io.Writer = os.Stdout
	consoleErr io.Writer = os.Stderr
)

// logURLSummary keeps protocol/host information useful for diagnosis without
// exposing credentials, signed query parameters, fragments, or path tokens.
func logURLSummary(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "<invalid-url>"
	}
	if parsed.Host == "" {
		return parsed.Scheme + ":<redacted>"
	}
	return parsed.Scheme + "://" + parsed.Host + "/<redacted>"
}

// Main runs the desktop command-line application.
func Main() {
	if isNativeMessagingInvocation(os.Args[1:]) {
		if err := runNativeHost(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "--bridge" {
		if err := runBridge(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	setupConsoleUTF8()
	consoleOut, consoleErr = consoleWriters()

	var err error
	if len(os.Args) >= 3 && os.Args[1] == "--worker" {
		err = runWorker(os.Args[2])
	} else {
		err = run()
	}
	if err != nil {
		fmt.Fprintln(consoleErr, "错误：", err)
		if runtime.GOOS == "windows" && isInteractiveInvocation() {
			fmt.Fprintln(consoleErr, "按回车关闭窗口……")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
		os.Exit(1)
	}
}

// Chromium starts a native messaging host with the caller origin as the first
// argument. Keep --native-host for direct protocol debugging, but do not require
// a wrapper executable merely to add that flag to the browser launch command.
func isNativeMessagingInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "--native-host" || strings.HasPrefix(args[0], "chrome-extension://")
}

func isInteractiveInvocation() bool {
	return isProtocolInvocation() || len(os.Args) >= 2 && os.Args[1] == "--worker"
}

func isProtocolInvocation() bool {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(strings.ToLower(arg), "downkit:") {
			return true
		}
	}
	return false
}

func run() error {
	opts, err := parseOptions()
	if err != nil {
		return err
	}
	return runWithOptions(opts, nil)
}

func runWithOptions(opts options, platformMuxer mediaMuxer) error {
	resolverPage, separatedMedia := pageForSeparatedMedia(opts.sourceURL, opts.referer, opts.resolvePage)
	directMP4 := isDirectMP4(opts.sourceURL)
	if separatedMedia && platformMuxer != nil {
		return errors.New("移动端第一版暂不支持由页面解析分离媒体轨，请发送到桌面版处理")
	}
	tools := map[string]*string{}
	if separatedMedia {
		tools["ffmpeg"] = &opts.ffmpegPath
		tools["yt-dlp"] = &opts.ytDLPPath
	} else if !directMP4 && platformMuxer == nil {
		tools["ffmpeg"] = &opts.ffmpegPath
	}
	for name, value := range tools {
		path, err := findTool(*value, name)
		if err != nil {
			return err
		}
		*value = path
	}
	if opts.ffmpegPath != "" {
		if err := validateFFmpegVersion(opts.ffmpegPath); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(opts.outputDir, 0o755); err != nil {
		return err
	}
	workRoot := filepath.Join(opts.outputDir, ".downkit-work")
	workName := fmt.Sprintf("job_%s_%d", time.Now().Format("20060102_150405"), os.Getpid())
	if opts.jobID != "" {
		workName = "job_" + opts.jobID
	}
	workDir := filepath.Join(workRoot, workName)
	segmentDir := filepath.Join(workDir, "segments")
	if err := os.MkdirAll(segmentDir, 0o755); err != nil {
		return err
	}
	// Credential adapters are always ephemeral, including when -keep-work is
	// enabled for media diagnostics.
	defer os.Remove(filepath.Join(workDir, "yt-dlp-task-cookies.txt"))
	defer os.Remove(filepath.Join(workDir, "curl-cookies.txt"))

	a := &app{
		opts:          opts,
		workDir:       workDir,
		segmentDir:    segmentDir,
		localPlaylist: filepath.Join(workDir, "local.m3u8"),
		muxer:         platformMuxer,
		mediaSession:  newMediaHTTPSession(opts),
	}
	if a.muxer == nil && !directMP4 {
		a.muxer = ffmpegMuxer{path: opts.ffmpegPath, stdout: os.Stdout, stderr: os.Stderr}
	}
	fmt.Fprintln(consoleOut, "任务：", opts.title)
	fmt.Fprintln(consoleOut, "来源：", logURLSummary(opts.sourceURL))
	fmt.Fprintln(consoleOut, "Origin：", logURLSummary(opts.origin))
	fmt.Fprintln(consoleOut, "Referer：", logURLSummary(opts.referer))
	fmt.Fprintln(consoleOut, "工作目录：", workDir)
	if a.opts.proxy != "" {
		fmt.Fprintf(consoleOut, "网络策略：全任务使用代理 %s\n", logURLSummary(a.opts.proxy))
	} else {
		fmt.Fprintln(consoleOut, "网络策略：仅直连")
	}
	if separatedMedia {
		publishJobPhaseProgress("resolving", 0, "正在准备解析", 0, 0, 0)
		publishJobPhaseProgress("resolving", 10, "正在解析媒体页面", 0, 0, 0)
		if err := a.downloadResolvedPage(resolverPage); err != nil {
			return keepWorkError(workDir, err)
		}
		if !opts.keepWork {
			_ = os.RemoveAll(workDir)
		}
		return nil
	}
	if directMP4 {
		outputPath, err := resumableOutputPath(workDir, opts.outputDir, opts.title)
		if err != nil {
			return keepWorkError(workDir, err)
		}
		haveCheckpoint := directCheckpointBytes(outputPath) > 0
		if haveCheckpoint {
			publishJobPhaseProgress("resolving", 25, "正在校验 MP4 续传点", directCheckpointBytes(outputPath), 0, 0)
		} else {
			publishJobPhaseProgress("resolving", 0, "正在准备解析", 0, 0, 0)
			publishJobPhaseProgress("resolving", 25, "正在探测媒体文件", 0, 0, 0)
		}
		fmt.Fprintln(consoleOut, "检测到 MP4 直链，使用 Go 内置下载器：", outputPath)
		if err := a.downloadDirectFile(opts.sourceURL, outputPath); err != nil {
			return keepWorkError(workDir, err)
		}
		publishJobOutput(outputPath)
		if !opts.keepWork {
			_ = os.RemoveAll(workDir)
		}
		fmt.Fprintln(consoleOut, "完成：", outputPath)
		return nil
	}
	if resumePlan, ok := loadHLSResumePlan(a); ok {
		fmt.Fprintln(consoleOut, "检测到已保存的 HLS 解析计划，跳过重新解析")
		return a.resumeHLS(resumePlan)
	}

	publishJobPhaseProgress("resolving", 0, "正在准备解析", 0, 0, 0)
	playlistURL := opts.sourceURL
	publishJobPhaseProgress("resolving", 20, "正在读取媒体清单", 0, 0, 0)
	playlist, err := a.fetchPlaylist(playlistURL, "playlist-0.m3u8")
	if err != nil {
		return keepWorkError(workDir, err)
	}

	var audioRendition *mediaRendition
	for depth := 0; depth < 5; depth++ {
		publishJobPhaseProgress("resolving", min(35+depth*10, 75), "正在选择媒体清晰度", 0, 0, 0)
		v, ok, err := selectVariant(playlist, playlistURL, opts.quality, opts.qualitySet)
		if err != nil {
			return keepWorkError(workDir, err)
		}
		if !ok {
			break
		}
		if v.audioGroup != "" {
			rendered, found, err := selectAudioRendition(playlist, playlistURL, v.audioGroup)
			if err != nil {
				return keepWorkError(workDir, err)
			}
			if found {
				audioRendition = &rendered
			}
		}
		fmt.Fprintf(consoleOut, "使用清晰度：%dp，带宽 %d bps\n", v.height, v.bandwidth)
		playlistURL = v.url
		playlist, err = a.fetchPlaylist(playlistURL, fmt.Sprintf("playlist-%d.m3u8", depth+1))
		if err != nil {
			return keepWorkError(workDir, err)
		}
	}
	if strings.Contains(playlist, "#EXT-X-STREAM-INF:") {
		return keepWorkError(workDir, errors.New("主清单嵌套超过 5 层"))
	}

	segments, err := a.prepareMediaPlaylist(playlist, playlistURL)
	if err != nil {
		return keepWorkError(workDir, err)
	}
	audioInput := ""
	var audioApp *app
	var audioSegments []segment
	if audioRendition != nil {
		publishJobPhaseProgress("resolving", 85, "正在解析独立音轨", 0, 0, 0)
		fmt.Fprintf(consoleOut, "获取独立音轨：%s（%s）\n", audioRendition.name, audioRendition.language)
		audioPlaylist, err := a.fetchPlaylist(audioRendition.url, "audio-source.m3u8")
		if err != nil {
			return keepWorkError(workDir, err)
		}
		preparedAudioApp := *a
		preparedAudioApp.workDir = filepath.Join(workDir, "audio")
		preparedAudioApp.segmentDir = filepath.Join(preparedAudioApp.workDir, "segments")
		preparedAudioApp.localPlaylist = filepath.Join(preparedAudioApp.workDir, "local.m3u8")
		if err := os.MkdirAll(preparedAudioApp.segmentDir, 0o755); err != nil {
			return keepWorkError(workDir, err)
		}
		audioSegments, err = preparedAudioApp.prepareMediaPlaylist(audioPlaylist, audioRendition.url)
		if err != nil {
			return keepWorkError(workDir, err)
		}
		audioApp = &preparedAudioApp
		audioInput = filepath.ToSlash(filepath.Join("audio", "local.m3u8"))
	}
	publishJobPhaseProgress("resolving", 100, "解析完成", 0, 0, 0)
	publishJobPhaseProgress("downloading", 0, "准备下载媒体分片", 0, 0, 0)
	if audioApp != nil {
		totalSegments := len(segments) + len(audioSegments)
		videoSpan := len(segments) * 100 / totalSegments
		a.segmentProgressSpan = videoSpan
		audioApp.segmentProgressStart = videoSpan
		audioApp.segmentProgressSpan = 100 - videoSpan
	}
	if err := saveHLSResumePlan(a, segments, audioSegments, audioInput); err != nil {
		return keepWorkError(workDir, err)
	}
	if err := a.downloadSegments(segments); err != nil {
		return keepWorkError(workDir, err)
	}

	if audioApp != nil {
		if err := audioApp.downloadSegments(audioSegments); err != nil {
			return keepWorkError(workDir, err)
		}
	}

	outputPath, err := resumableOutputPath(workDir, opts.outputDir, opts.title)
	if err != nil {
		return keepWorkError(workDir, err)
	}
	publishJobPhaseProgress("processing", 0, "正在封装 MP4", 0, 0, 0)
	if err := a.mux(outputPath, audioInput); err != nil {
		return keepWorkError(workDir, err)
	}
	publishJobOutput(outputPath)

	if !opts.keepWork {
		if err := os.RemoveAll(workDir); err != nil {
			fmt.Fprintln(consoleOut, "警告：无法清理工作目录：", err)
		}
	}
	fmt.Fprintln(consoleOut, "完成：", outputPath)
	return nil
}

func resumableOutputPath(workDir, outputDir, title string) (string, error) {
	statePath := filepath.Join(workDir, "output-path.txt")
	if data, err := os.ReadFile(statePath); err == nil {
		if saved := strings.TrimSpace(string(data)); saved != "" {
			return saved, nil
		}
	}
	outputPath := uniqueOutput(outputDir, mp4OutputName(title))
	if err := os.WriteFile(statePath, []byte(outputPath), 0o600); err != nil {
		return "", err
	}
	return outputPath, nil
}

func parseOptions() (options, error) {
	home, _ := os.UserHomeDir()
	defaultOutput := filepath.Join(home, "Downloads")

	var o options
	var qualityArg string
	var headersArg string
	flag.StringVar(&o.sourceURL, "url", "", "m3u8 URL")
	flag.StringVar(&o.title, "title", "", "输出文件名")
	flag.StringVar(&o.referer, "referer", "", "Referer 请求头")
	flag.StringVar(&o.origin, "origin", "", "Origin 请求头")
	flag.StringVar(&o.userAgent, "ua", defaultUA, "User-Agent")
	flag.StringVar(&headersArg, "headers-json", "", "附加请求头 JSON 对象")
	flag.StringVar(&o.proxy, "proxy", "", "HTTP 代理；配置后整项任务使用该线路，留空表示直连")
	flag.StringVar(&o.outputDir, "output-dir", defaultOutput, "输出目录")
	flag.StringVar(&o.ffmpegPath, "ffmpeg", "", "ffmpeg 路径")
	flag.StringVar(&o.ytDLPPath, "yt-dlp", "", "yt-dlp 路径")
	flag.StringVar(&o.playlistMode, "playlist", "ask", "页面播放列表模式：ask、single 或 all")
	flag.StringVar(&qualityArg, "quality", "", "best 或目标高度，例如 720；省略时交互选择")
	flag.IntVar(&o.limit, "limit", 0, "仅下载前 N 个分片，用于测试")
	flag.IntVar(&o.concurrent, "concurrent", 12, "Go 内置下载器的最大并发数")
	flag.BoolVar(&o.keepWork, "keep-work", false, "成功后保留工作目录")
	flag.BoolVar(&o.resolvePage, "resolve-page", false, "使用 yt-dlp 解析来源页面")
	flag.Parse()
	if headersArg != "" {
		headers, err := parseRequestHeaders(headersArg)
		if err != nil {
			return o, fmt.Errorf("无效 -headers-json：%w", err)
		}
		o.requestHeaders = headers
	}
	if qualityArg != "" {
		if err := setQuality(&o, qualityArg); err != nil {
			return o, err
		}
	}

	if o.sourceURL == "" && flag.NArg() > 0 {
		arg := flag.Arg(0)
		if strings.HasPrefix(strings.ToLower(arg), "downkit:") {
			if err := applyProtocolURI(&o, arg); err != nil {
				return o, err
			}
		} else {
			o.sourceURL = arg
		}
	}
	return normalizeOptions(o)
}

func normalizeOptions(o options) (options, error) {
	if o.sourceURL == "" {
		return o, errors.New("缺少 -url 或 downkit: 参数")
	}
	o.sourceURL = cleanURL(o.sourceURL)
	proxy, _, _, proxyErr := normalizeProxyAddress(o.proxy, "", 0)
	if proxyErr != nil {
		return o, proxyErr
	}
	o.proxy = proxy
	o.referer = cleanURL(o.referer)
	o.origin = cleanURL(o.origin)
	if originURL, err := url.Parse(o.origin); err == nil && originURL.Scheme != "" && originURL.Host != "" {
		o.origin = originURL.Scheme + "://" + originURL.Host
	}
	u, err := url.Parse(o.sourceURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return o, errors.New("只允许绝对 HTTP/HTTPS URL")
	}
	if o.title == "" {
		o.title = "video_" + time.Now().Format("20060102_150405")
	}
	if o.origin == "" && o.referer != "" {
		if ref, err := url.Parse(o.referer); err == nil {
			o.origin = ref.Scheme + "://" + ref.Host
		}
	}
	if o.concurrent < 1 {
		o.concurrent = 1
	}
	if o.limit < 0 {
		o.limit = 0
	}
	o.playlistMode, err = normalizePlaylistMode(o.playlistMode)
	if err != nil {
		return o, err
	}
	return o, nil
}

func applyProtocolURI(o *options, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	q := u.Query()
	o.sourceURL = q.Get("url")
	if v := q.Get("title"); v != "" {
		o.title = v
	}
	if v := q.Get("referer"); v != "" {
		o.referer = v
	}
	if v := q.Get("origin"); v != "" {
		o.origin = v
	}
	if v := q.Get("ua"); v != "" {
		o.userAgent = v
	}
	if v := q.Get("headers"); v != "" {
		headers, err := parseRequestHeaders(v)
		if err != nil {
			return fmt.Errorf("无效 headers 参数：%w", err)
		}
		o.requestHeaders = headers
	}
	if v := q.Get("playlist"); v != "" {
		o.playlistMode = v
	}
	if v := q.Get("resolve"); strings.EqualFold(v, "page") {
		o.resolvePage = true
	}
	if values, ok := q["proxy"]; ok && len(values) > 0 {
		o.proxy = values[0]
	}
	if v := q.Get("quality"); v != "" {
		if err := setQuality(o, v); err != nil {
			return err
		}
	}
	if v := q.Get("limit"); v != "" {
		o.limit, _ = strconv.Atoi(v)
	}
	return nil
}

func parseRequestHeaders(raw string) (map[string]string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(values))
	for name, value := range values {
		name = strings.TrimSpace(name)
		lower := strings.ToLower(name)
		if !validRequestHeaderName(name) || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("非法请求头 %q", name)
		}
		switch lower {
		case "accept-encoding", "connection", "content-length", "cookie", "host", "proxy-authorization", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
			continue
		}
		result[http.CanonicalHeaderKey(name)] = value
	}
	return result, nil
}

func validRequestHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		}
		return false
	}
	return true
}

func applyMediaRequestHeaders(req *http.Request, opts options) {
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")
	if opts.userAgent != "" {
		req.Header.Set("User-Agent", opts.userAgent)
	}
	if opts.referer != "" {
		req.Header.Set("Referer", opts.referer)
	}
	if opts.origin != "" {
		req.Header.Set("Origin", opts.origin)
	}
	for name, value := range opts.requestHeaders {
		if sensitiveRequestHeader(name) && !sameURLHost(opts.sourceURL, req.URL) {
			continue
		}
		req.Header.Set(name, value)
	}
}

func sensitiveRequestHeader(name string) bool {
	return strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Authorization")
}

func sameURLHost(source string, target *url.URL) bool {
	parsed, err := url.Parse(source)
	if err != nil || target == nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), target.Hostname()) && effectivePort(parsed) == effectivePort(target)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	return ""
}

func normalizePlaylistMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = "ask"
	}
	switch mode {
	case "ask", "single", "all":
		return mode, nil
	default:
		return "", fmt.Errorf("无效的 playlist：%s（应为 ask、single 或 all）", raw)
	}
}

func setQuality(o *options, raw string) error {
	value := strings.ToLower(strings.TrimSpace(raw))
	o.qualitySet = true
	if value == "best" || value == "0" {
		o.quality = 0
		return nil
	}
	height, err := strconv.Atoi(strings.TrimSuffix(value, "p"))
	if err != nil || height < 1 {
		return fmt.Errorf("无效的 quality：%s（应为 best、720 或 720p）", raw)
	}
	o.quality = height
	return nil
}

func cleanURL(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	cleaned = strings.Trim(cleaned, "\"'“”‘’")
	cleaned = strings.TrimRight(cleaned, "，。")
	return strings.TrimSpace(cleaned)
}

func isDirectMP4(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(filepath.Ext(u.Path), ".mp4")
}

func pageForSeparatedMedia(source, referer string, forcePage bool) (string, bool) {
	sourceURL, err := url.Parse(source)
	if err != nil || (sourceURL.Scheme != "http" && sourceURL.Scheme != "https") {
		return "", false
	}
	if forcePage {
		sourceURL.Fragment = ""
		return sourceURL.String(), true
	}
	ext := strings.ToLower(filepath.Ext(sourceURL.Path))
	if ext != ".m4s" && ext != ".mpd" {
		return "", false
	}
	pageURL, err := url.Parse(referer)
	if err != nil || (pageURL.Scheme != "http" && pageURL.Scheme != "https") || pageURL.Host == "" {
		sourceURL.Fragment = ""
		return sourceURL.String(), true
	}
	pageURL.Fragment = ""
	return pageURL.String(), true
}

func mp4OutputName(title string) string {
	name := safeName(title)
	if strings.EqualFold(filepath.Ext(name), ".mp4") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name + ".mp4"
}

func toolExecutableNames(name string) []string {
	toolName := name
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(toolName), ".exe") {
		toolName += ".exe"
	}
	if name != "ffmpeg" {
		return []string{toolName}
	}
	slimName := "ffmpeg-slim"
	if runtime.GOOS == "windows" {
		slimName += ".exe"
	}
	return []string{slimName, toolName}
}

func findTool(preferred, name string) (string, error) {
	if preferred != "" {
		if _, err := os.Stat(preferred); err == nil {
			return preferred, nil
		}
		return "", fmt.Errorf("找不到 %s：%s", name, preferred)
	}
	toolNames := toolExecutableNames(name)
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates := make([]string, 0, len(toolNames)*2)
		for _, toolDir := range []string{filepath.Join(executableDir, "tools"), executableDir} {
			for _, candidateName := range toolNames {
				candidates = append(candidates, filepath.Join(toolDir, candidateName))
			}
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if configTools, err := bridgeDataPath("tools"); err == nil {
		for _, candidateName := range toolNames {
			candidate := filepath.Join(configTools, candidateName)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("找不到 %s，请通过对应参数指定路径", name)
}

type ffmpegVersion struct {
	major            int
	minor            int
	development      bool
	developmentKind  string
	developmentOrder int
	label            string
}

func parseFFmpegVersion(output string) (ffmpegVersion, error) {
	firstLine := strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
	version := ffmpegVersion{label: firstLine}
	if match := ffmpegDatedVersionRE.FindStringSubmatch(firstLine); len(match) == 4 {
		version.development = true
		version.developmentKind = "date"
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		day, _ := strconv.Atoi(match[3])
		version.developmentOrder = year*10000 + month*100 + day
		return version, nil
	}
	if match := ffmpegNVersionRE.FindStringSubmatch(firstLine); len(match) == 2 {
		version.development = true
		version.developmentKind = "build"
		version.developmentOrder, _ = strconv.Atoi(match[1])
		return version, nil
	}
	match := ffmpegStableVersionRE.FindStringSubmatch(firstLine)
	if len(match) != 3 {
		return version, fmt.Errorf("无法识别 FFmpeg 版本：%s", firstLine)
	}
	version.major, _ = strconv.Atoi(match[1])
	version.minor, _ = strconv.Atoi(match[2])
	return version, nil
}

func checkFFmpegMinimum(version ffmpegVersion) error {
	if version.development {
		supported := version.developmentKind == "date" && version.developmentOrder >= 20210408 ||
			version.developmentKind == "build" && version.developmentOrder >= 102000
		if !supported {
			return fmt.Errorf("FFmpeg 开发版过旧或无法确认达到 4.4：%s", version.label)
		}
		return nil
	}
	if version.major < 4 || version.major == 4 && version.minor < 4 {
		return fmt.Errorf("FFmpeg 版本过低：%s；DownKit 最低要求 FFmpeg 4.4", version.label)
	}
	return nil
}

func validateFFmpegVersion(path string) error {
	output, err := exec.Command(path, "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("无法运行 FFmpeg %s：%w", path, err)
	}
	version, err := parseFFmpegVersion(string(output))
	if err != nil {
		return err
	}
	if err := checkFFmpegMinimum(version); err != nil {
		return err
	}
	fmt.Fprintf(consoleOut, "FFmpeg：%s（%s）\n", path, version.label)
	return nil
}

type ytDLPAttempt struct {
	proxy          string
	taskCookiePath string
}

type ytDLPPlaylistInfo struct {
	Type          string            `json:"_type"`
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	PlaylistCount int               `json:"playlist_count"`
	Entries       []json.RawMessage `json:"entries"`
}

type ytDLPPlaylistEntry struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	PlaylistIndex int    `json:"playlist_index"`
}

func (p ytDLPPlaylistInfo) count() int {
	if p.PlaylistCount > 0 {
		return p.PlaylistCount
	}
	if len(p.Entries) > 0 {
		return len(p.Entries)
	}
	return 1
}

func (p ytDLPPlaylistInfo) jobFiles() []bridgeJobFile {
	files := make([]bridgeJobFile, 0, len(p.Entries))
	for position, raw := range p.Entries {
		var entry ytDLPPlaylistEntry
		if json.Unmarshal(raw, &entry) != nil {
			continue
		}
		index := entry.PlaylistIndex
		if index <= 0 {
			index = position + 1
		}
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			title = fmt.Sprintf("第 %d 个视频", index)
		}
		files = append(files, bridgeJobFile{Index: index, ID: strings.TrimSpace(entry.ID), Title: title, Status: "pending"})
	}
	return files
}

func choosePlaylistMode(input io.Reader, output io.Writer, count int) string {
	fmt.Fprintf(output, "检测到视频选集，共 %d 集：\n", count)
	fmt.Fprintln(output, "  1) 仅下载当前一集")
	fmt.Fprintf(output, "  2) 批量下载全部 %d 集\n", count)
	reader := bufio.NewReader(input)
	for {
		fmt.Fprint(output, "请选择 [1-2]，直接回车默认 1：")
		line, err := reader.ReadString('\n')
		switch strings.TrimSpace(line) {
		case "", "1":
			return "single"
		case "2":
			return "all"
		default:
			fmt.Fprintln(output, "输入无效，请重新输入。")
		}
		if err != nil {
			fmt.Fprintln(output, "无法读取输入，自动选择当前一集。")
			return "single"
		}
	}
}

func (a *app) downloadResolvedPage(pageURL string) error {
	access, err := a.prepareYTDLPAccess(pageURL)
	if err != nil {
		return err
	}
	defer access.cleanup()
	fmt.Fprintln(consoleOut, "检测到需要从来源页面解析的分离媒体资源。")
	fmt.Fprintln(consoleOut, "改由 yt-dlp 进行通用页面能力探测：", logURLSummary(pageURL))
	if access.taskCookiePath != "" {
		fmt.Fprintln(consoleOut, "登录态：使用浏览器扩展为当前任务传入的结构化 Cookie。")
	} else {
		fmt.Fprintln(consoleOut, "登录态：扩展未为当前页面传入 Cookie。")
	}
	attempts := access.attempts

	playlistURL := pageURL
	publishJobPhaseProgress("resolving", 30, "正在检测页面播放列表", 0, 0, 0)
	playlistInfo, preferredAttempt, probeErr := a.probeYTDLPPlaylist(playlistURL, attempts)
	playlistCount := 1
	if probeErr != nil {
		fmt.Fprintln(consoleOut, "警告：无法预先检测页面播放列表，将按单项继续：", probeErr)
	} else {
		playlistCount = playlistInfo.count()
		if playlistInfo.Type == "playlist" && playlistCount > 1 {
			fmt.Fprintf(consoleOut, "选集检测：%d 集\n", playlistCount)
		}
		if preferredAttempt > 0 {
			ordered := make([]ytDLPAttempt, 0, len(attempts))
			ordered = append(ordered, attempts[preferredAttempt])
			for i, attempt := range attempts {
				if i != preferredAttempt {
					ordered = append(ordered, attempt)
				}
			}
			attempts = ordered
		}
	}

	mode := a.opts.playlistMode
	if mode == "ask" {
		mode = "single"
		if playlistCount > 1 {
			mode = choosePlaylistMode(os.Stdin, consoleOut, playlistCount)
		}
	}
	downloadAll := mode == "all"
	if downloadAll {
		publishJobFiles(playlistInfo.jobFiles())
	}
	downloadURL := pageURL
	var outputTemplate string
	if downloadAll {
		downloadURL = playlistURL
		folder, template := playlistBatchOutputTemplate(a.opts.title, playlistInfo.ID)
		outputTemplate = template
		if playlistCount > 1 {
			fmt.Fprintf(consoleOut, "批量模式：将下载全部 %d 集到目录：%s\n", playlistCount, filepath.Join(a.opts.outputDir, folder))
		} else {
			fmt.Fprintln(consoleOut, "批量模式：由 yt-dlp 下载页面中的全部选集。")
		}
	} else {
		outputPath := uniqueOutput(a.opts.outputDir, mp4OutputName(a.opts.title))
		outputBase := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))
		outputTemplate = strings.ReplaceAll(outputBase, "%", "%%") + ".%(ext)s"
		if playlistCount > 1 {
			fmt.Fprintln(consoleOut, "单集模式：仅下载当前播放的一集。")
		}
	}

	var lastErr error
	outputRecord := filepath.Join(a.workDir, "yt-dlp-outputs.txt")
	_ = os.Remove(outputRecord)
	publishJobPhaseProgress("resolving", 100, "解析完成", 0, 0, 0)
	publishJobPhaseProgress("downloading", 0, "准备下载页面媒体", 0, 0, 0)
	for i, attempt := range attempts {
		if i > 0 {
			mode := "直连"
			if attempt.proxy != "" {
				mode = "代理 " + attempt.proxy
			}
			fmt.Fprintf(consoleOut, "yt-dlp 改用%s重试。\n", mode)
		}
		if err := a.runYTDLP(downloadURL, outputTemplate, outputRecord, downloadAll, attempt); err == nil {
			a.publishRecordedOutputs(outputRecord)
			return nil
		} else {
			lastErr = err
			fmt.Fprintln(consoleErr, "yt-dlp 本轮失败：", err)
		}
	}
	return fmt.Errorf("yt-dlp 的直连/代理尝试均失败：%w", lastErr)
}

func writeTaskCookieFile(workDir, filename string, cookies []browserCookie) (string, error) {
	var content strings.Builder
	content.WriteString("# Netscape HTTP Cookie File\n")
	for _, cookie := range sanitizeBrowserCookies(cookies) {
		domain := cookie.Domain
		includeSubdomains := !cookie.HostOnly || strings.HasPrefix(domain, ".")
		if includeSubdomains && !strings.HasPrefix(domain, ".") {
			domain = "." + domain
		}
		if cookie.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		expires := int64(0)
		if !cookie.Session && cookie.ExpirationDate > 0 {
			expires = int64(cookie.ExpirationDate)
		}
		fmt.Fprintf(&content, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			domain, netscapeBool(includeSubdomains), cookie.Path, netscapeBool(cookie.Secure), expires, cookie.Name, cookie.Value)
	}
	path := filepath.Join(workDir, filename)
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func netscapeBool(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}

func (a *app) probeYTDLPPlaylist(pageURL string, attempts []ytDLPAttempt) (ytDLPPlaylistInfo, int, error) {
	var lastErr error
	for i, attempt := range attempts {
		args := []string{"--no-color", "--no-warnings", "--flat-playlist", "--dump-single-json", "--yes-playlist"}
		args = a.appendYTDLPAccessArgs(args, pageURL, attempt)
		args = append(args, pageURL)

		cmd := exec.Command(a.opts.ytDLPPath, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			message := strings.TrimSpace(stderr.String())
			if len(message) > 300 {
				message = message[len(message)-300:]
			}
			lastErr = fmt.Errorf("%w: %s", err, message)
			continue
		}
		var info ytDLPPlaylistInfo
		if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
			lastErr = fmt.Errorf("yt-dlp 选集信息无法解析：%w", err)
			continue
		}
		return info, i, nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的 yt-dlp 网络尝试")
	}
	return ytDLPPlaylistInfo{}, -1, lastErr
}

func (a *app) appendYTDLPAccessArgs(args []string, pageURL string, attempt ytDLPAttempt) []string {
	if attempt.proxy == "" {
		args = append(args, "--proxy=")
	} else {
		args = append(args, "--proxy", attempt.proxy)
	}
	if attempt.taskCookiePath != "" {
		args = append(args, "--cookies", attempt.taskCookiePath)
	}
	if a.opts.userAgent != "" {
		args = append(args, "--user-agent", a.opts.userAgent)
	}
	return append(args, "--referer", pageURL)
}

func (a *app) runYTDLP(pageURL, outputTemplate, outputRecord string, downloadAll bool, attempt ytDLPAttempt) error {
	args := a.ytDLPDownloadArgs(pageURL, outputTemplate, outputRecord, downloadAll, attempt)
	err, detail := a.runYTDLPCommand(args)
	if err == nil {
		return nil
	}
	for _, fallback := range ytDLPFallbacks(detail) {
		fmt.Fprintln(consoleOut, fallback.message)
		retryArgs := insertYTDLPArgsBeforeURL(args, fallback.args...)
		if retryErr, retryDetail := a.runYTDLPCommand(retryArgs); retryErr == nil {
			return nil
		} else {
			err, detail = retryErr, retryDetail
		}
	}
	return describeYTDLPFailure(err, detail)
}

func describeYTDLPFailure(err error, detail string) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "[bilibili]") && (strings.Contains(lower, "http error 412") || strings.Contains(lower, "precondition failed")) {
		return fmt.Errorf("B站拒绝了页面解析请求（HTTP 412），可能是当前代理出口或匿名会话触发风控；DownKit 已尝试当前线路，若仍失败请稍后重试：%w", err)
	}
	message := strings.TrimSpace(detail)
	if len(message) > 500 {
		message = message[len(message)-500:]
	}
	if message == "" {
		return err
	}
	return fmt.Errorf("%w：%s", err, message)
}

func (a *app) runYTDLPCommand(args []string) (error, string) {
	cmd := exec.Command(a.opts.ytDLPPath, args...)
	stdout := newYTDLPProgressWriter(consoleOut)
	var errorOutput tailBuffer
	errorOutput.limit = 64 * 1024
	stderr := newYTDLPProgressWriter(io.MultiWriter(consoleErr, &errorOutput))
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	stdout.Flush()
	stderr.Flush()
	return err, errorOutput.String()
}

type ytDLPFallback struct {
	message string
	args    []string
}

func ytDLPFallbacks(detail string) []ytDLPFallback {
	match := ytDLPExtractorErrorRE.FindStringSubmatch(detail)
	if len(match) != 2 || !strings.Contains(strings.ToLower(detail), "requested format is not available") {
		return nil
	}
	extractor := strings.ToLower(strings.TrimSpace(strings.SplitN(match[1], ":", 2)[0]))
	// Select by yt-dlp extractor identity instead of URL host. This covers
	// redirects and alternate front ends handled by the same extractor while
	// keeping extractor-specific flags away from unrelated sites.
	switch extractor {
	case "youtube":
		return []ytDLPFallback{{
			message: "yt-dlp 未找到可下载格式，改用 YouTube HLS 兼容模式重试。",
			args:    []string{"--extractor-args", "youtube:player_client=web_safari"},
		}}
	default:
		return nil
	}
}

func insertYTDLPArgsBeforeURL(args []string, extra ...string) []string {
	if len(args) == 0 {
		return append([]string(nil), extra...)
	}
	result := make([]string, 0, len(args)+len(extra))
	result = append(result, args[:len(args)-1]...)
	result = append(result, extra...)
	return append(result, args[len(args)-1])
}

type tailBuffer struct {
	data  []byte
	limit int
}

func (b *tailBuffer) Write(data []byte) (int, error) {
	written := len(data)
	if b.limit <= 0 {
		return written, nil
	}
	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.data = append(b.data[:0], b.data[len(b.data)-b.limit:]...)
	}
	return written, nil
}

func (b *tailBuffer) String() string {
	return string(b.data)
}

func (a *app) ytDLPDownloadArgs(pageURL, outputTemplate, outputRecord string, downloadAll bool, attempt ytDLPAttempt) []string {
	archive := filepath.Join(a.workDir, "yt-dlp-archive.txt")
	args := []string{
		"--no-color", "--newline", "--progress", "--windows-filenames",
		"--continue", "--download-archive", archive,
		"--progress-delta", "1",
		"--progress-template", "download:downkit-progress:%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress._percent_str)s",
		"--progress-template", "postprocess:downkit-postprocess:%(info.id)s",
		"--no-mtime", "--retries", "5", "--fragment-retries", "5",
		"--extractor-retries", "3", "--socket-timeout", "20",
		"--concurrent-fragments", strconv.Itoa(a.opts.concurrent),
		"--format", "bv*+ba/b", "--merge-output-format", "mp4",
		"--ffmpeg-location", a.opts.ffmpegPath,
		"--paths", a.opts.outputDir, "--output", outputTemplate,
		"--print", "after_move:downkit-output:%(playlist_index)s|%(id)s|%(filepath)s",
		"--print-to-file", "after_move:%(filepath)s", strings.ReplaceAll(outputRecord, "%", "%%"),
	}
	if downloadAll {
		args = append(args, "--yes-playlist", "--trim-filenames", "100")
	} else {
		args = append(args, "--no-playlist")
	}
	args = a.appendYTDLPAccessArgs(args, pageURL, attempt)
	args = append(args, pageURL)
	return args
}

func (a *app) publishRecordedOutputs(record string) {
	data, err := os.ReadFile(record)
	if err != nil {
		fmt.Fprintln(consoleErr, "警告：无法读取 yt-dlp 输出文件记录：", err)
		return
	}
	seen := make(map[string]bool)
	for _, line := range splitLines(string(data)) {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.opts.outputDir, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		publishJobOutput(path)
	}
}

func (a *app) fetchPlaylist(sourceURL, name string) (string, error) {
	path := filepath.Join(a.workDir, name)
	fmt.Fprintln(consoleOut, "获取清单：", logURLSummary(sourceURL))
	var data []byte
	var status int
	var err error
	if a.useCurlHTTP {
		data, status, err = a.fetchHTTPCurl(sourceURL)
	} else {
		data, status, err = a.fetchHTTP(sourceURL)
		if err != nil && status == http.StatusForbidden {
			compatData, compatStatus, compatErr := a.fetchHTTPCurl(sourceURL)
			if compatErr == nil {
				a.useCurlHTTP = true
				data, status, err = compatData, compatStatus, nil
				fmt.Fprintln(consoleOut, "Go HTTP 被服务器拒绝，当前任务已切换到 curl 兼容传输。")
			}
		}
	}
	if err != nil {
		switch status {
		case http.StatusNotFound:
			return "", fmt.Errorf("HTTP 404：源清单不存在或已下线，请刷新网页后重新抓取（URL=%q）", sourceURL)
		case http.StatusForbidden:
			return "", fmt.Errorf("HTTP 403：服务器拒绝访问，可能是会话已失效、防盗链、地区限制或签名过期；请求上下文：%s。请刷新来源页面并重新嗅探后再试（URL=%q）", accessContextSummary(a.opts), sourceURL)
		default:
			if status >= 400 && status < 500 {
				return "", fmt.Errorf("HTTP %d：请求被服务器拒绝（URL=%q）", status, sourceURL)
			}
			return "", fmt.Errorf("Go HTTP 获取清单失败（URL=%q）：%w", sourceURL, err)
		}
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	if !strings.HasPrefix(strings.TrimSpace(text), "#EXTM3U") {
		return "", fmt.Errorf("返回内容不是 M3U8：%s", responsePreview(data))
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return text, nil
}

func (a *app) fetchHTTP(sourceURL string) ([]byte, int, error) {
	transport, err := proxyHTTPTransport(a.opts.proxy)
	if err != nil {
		return nil, 0, err
	}
	client := a.mediaHTTPClient(transport, 120*time.Second)
	defer transport.CloseIdleConnections()

	var lastErr error
	var lastStatus int
	for attempt := 1; attempt <= 5; attempt++ {
		req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
		if err != nil {
			return nil, 0, err
		}
		a.applyTaskMediaRequestHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
			_ = resp.Body.Close()
			lastStatus = resp.StatusCode
			if readErr != nil {
				lastErr = readErr
			} else if len(body) > 16<<20 {
				return nil, resp.StatusCode, errors.New("响应超过 16 MiB 限制")
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return body, resp.StatusCode, nil
			} else {
				lastErr = fmt.Errorf("HTTP %s：%s", resp.Status, responsePreview(body))
				if !retryableHTTPStatus(resp.StatusCode) {
					return nil, resp.StatusCode, lastErr
				}
			}
		}
		if attempt < 5 {
			time.Sleep(time.Second)
		}
	}
	return nil, lastStatus, lastErr
}

func retryableHTTPStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func responsePreview(data []byte) string {
	text := strings.TrimSpace(string(data))
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > 200 {
		text = string([]rune(text)[:200]) + "…"
	}
	if text == "" {
		return "空响应"
	}
	return text
}

func formatHTTPStatus(status int) string {
	if status > 0 {
		return fmt.Sprintf("（HTTP %d）", status)
	}
	return ""
}

func selectVariant(text, base string, quality int, qualitySet bool) (variant, bool, error) {
	lines := splitLines(text)
	variants := make([]variant, 0)
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "#EXT-X-STREAM-INF:") {
			continue
		}
		var v variant
		if m := bandwidthRE.FindStringSubmatch(line); len(m) == 2 {
			v.bandwidth, _ = strconv.ParseInt(m[1], 10, 64)
		}
		if m := heightRE.FindStringSubmatch(line); len(m) == 2 {
			v.height, _ = strconv.Atoi(m[1])
		}
		v.audioGroup = parseHLSAttributes(line)["AUDIO"]
		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" || strings.HasPrefix(candidate, "#") {
				continue
			}
			resolved, err := resolveURL(base, candidate)
			if err != nil {
				return variant{}, false, err
			}
			v.url = resolved
			variants = append(variants, v)
			break
		}
	}
	if len(variants) == 0 {
		return variant{}, false, nil
	}
	sort.Slice(variants, func(i, j int) bool {
		if variants[i].height != variants[j].height {
			return variants[i].height > variants[j].height
		}
		return variants[i].bandwidth > variants[j].bandwidth
	})
	if !qualitySet && len(variants) > 1 {
		return promptVariant(variants), true, nil
	}
	if quality > 0 {
		for _, v := range variants {
			if v.height == quality {
				return v, true, nil
			}
		}
		fmt.Fprintf(consoleOut, "没有找到 %dp，改选最高码率\n", quality)
	}
	return variants[0], true, nil
}

func audioRenditions(text, base, groupID string) ([]mediaRendition, error) {
	result := make([]mediaRendition, 0)
	for _, line := range splitLines(text) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), "#EXT-X-MEDIA:") {
			continue
		}
		attributes := parseHLSAttributes(trimmed)
		if !strings.EqualFold(attributes["TYPE"], "AUDIO") || attributes["GROUP-ID"] != groupID || attributes["URI"] == "" {
			continue
		}
		resolved, err := resolveURL(base, attributes["URI"])
		if err != nil {
			return nil, err
		}
		name := attributes["NAME"]
		if name == "" {
			name = attributes["LANGUAGE"]
		}
		if name == "" {
			name = "未命名音轨"
		}
		result = append(result, mediaRendition{
			url:        resolved,
			name:       name,
			language:   attributes["LANGUAGE"],
			isDefault:  strings.EqualFold(attributes["DEFAULT"], "YES"),
			autoSelect: strings.EqualFold(attributes["AUTOSELECT"], "YES"),
		})
	}
	return result, nil
}

func selectAudioRendition(text, base, groupID string) (mediaRendition, bool, error) {
	renditions, err := audioRenditions(text, base, groupID)
	if err != nil || len(renditions) == 0 {
		return mediaRendition{}, false, err
	}
	if len(renditions) == 1 {
		return renditions[0], true, nil
	}
	return promptAudioRendition(renditions), true, nil
}

func promptAudioRendition(renditions []mediaRendition) mediaRendition {
	defaultIndex := 0
	for i, rendition := range renditions {
		if rendition.isDefault {
			defaultIndex = i
			break
		}
	}
	fmt.Fprintf(consoleOut, "检测到 %d 个独立音轨，请选择：\n", len(renditions))
	for i, rendition := range renditions {
		language := rendition.language
		if language == "" {
			language = "未知语言"
		}
		marker := ""
		if i == defaultIndex {
			marker = "（默认）"
		}
		fmt.Fprintf(consoleOut, "  %d) %s · %s%s\n", i+1, rendition.name, language, marker)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(consoleOut, "请输入序号 [1-%d]，直接回车默认 %d：", len(renditions), defaultIndex+1)
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return renditions[defaultIndex]
		}
		choice, parseErr := strconv.Atoi(line)
		if parseErr == nil && choice >= 1 && choice <= len(renditions) {
			return renditions[choice-1]
		}
		fmt.Fprintln(consoleOut, "输入无效，请重新输入。")
		if err != nil {
			return renditions[defaultIndex]
		}
	}
}

func promptVariant(variants []variant) variant {
	fmt.Fprintf(consoleOut, "检测到 %d 个清晰度，请选择：\n", len(variants))
	for i, v := range variants {
		label := "未知分辨率"
		if v.height > 0 {
			label = fmt.Sprintf("%dp", v.height)
		}
		bandwidth := "未知码率"
		if v.bandwidth > 0 {
			bandwidth = fmt.Sprintf("%.2f Mbps", float64(v.bandwidth)/1_000_000)
		}
		best := ""
		if i == 0 {
			best = "（最高）"
		}
		fmt.Fprintf(consoleOut, "  %d) %s · %s%s\n", i+1, label, bandwidth, best)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(consoleOut, "请输入序号 [1-%d]，直接回车默认 1：", len(variants))
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			if err != nil {
				fmt.Fprintln(consoleOut, "无法读取输入，自动选择最高清晰度。")
			}
			return variants[0]
		}
		choice, parseErr := strconv.Atoi(line)
		if parseErr == nil && choice >= 1 && choice <= len(variants) {
			return variants[choice-1]
		}
		fmt.Fprintln(consoleOut, "输入无效，请重新输入。")
		if err != nil {
			return variants[0]
		}
	}
}

func parseHLSAttributes(line string) map[string]string {
	if colon := strings.IndexByte(line, ':'); colon >= 0 {
		line = line[colon+1:]
	}
	parts := make([]string, 0)
	start := 0
	quoted := false
	for i, r := range line {
		switch r {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				parts = append(parts, line[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, line[start:])
	attributes := make(map[string]string, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), "\"")
		if key != "" {
			attributes[key] = value
		}
	}
	return attributes
}

func parseByteRange(raw string, implicitStart int64, allowImplicit bool) (int64, int64, error) {
	raw = strings.Trim(strings.TrimSpace(raw), "\"")
	lengthText, offsetText, hasOffset := strings.Cut(raw, "@")
	length, err := strconv.ParseInt(strings.TrimSpace(lengthText), 10, 64)
	if err != nil || length <= 0 {
		return 0, 0, fmt.Errorf("无效的 BYTERANGE 长度：%q", raw)
	}
	start := implicitStart
	if hasOffset {
		start, err = strconv.ParseInt(strings.TrimSpace(offsetText), 10, 64)
		if err != nil || start < 0 {
			return 0, 0, fmt.Errorf("无效的 BYTERANGE 偏移：%q", raw)
		}
	} else if !allowImplicit {
		return 0, 0, fmt.Errorf("BYTERANGE %q 缺少起始偏移", raw)
	}
	return start, length, nil
}

func (a *app) prepareMediaPlaylist(text, base string) ([]segment, error) {
	lines := splitLines(text)
	isFMP4 := false
	for _, line := range lines {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "#EXT-X-MAP:") {
			isFMP4 = true
			break
		}
	}
	indices := make([]int, 0)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		return nil, errors.New("媒体清单中没有分片")
	}
	if a.opts.limit > 0 && a.opts.limit < len(indices) {
		indices = indices[:a.opts.limit]
		lines = append(lines[:indices[len(indices)-1]+1], "#EXT-X-ENDLIST")
		fmt.Fprintf(consoleOut, "测试模式：只下载前 %d 个分片\n", a.opts.limit)
	}

	indexSet := make(map[int]bool, len(indices))
	for _, i := range indices {
		indexSet[i] = true
	}
	segments := make([]segment, 0, len(indices)+1)
	keyNames := make(map[string]string)
	keyIndex := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), "#EXT-X-KEY:") {
			continue
		}
		methodMatch := methodRE.FindStringSubmatch(trimmed)
		if len(methodMatch) != 2 {
			return nil, fmt.Errorf("无法解析 HLS 加密方式：%s", trimmed)
		}
		method := strings.ToUpper(strings.TrimSpace(methodMatch[1]))
		if method == "NONE" {
			continue
		}
		if method != "AES-128" {
			return nil, fmt.Errorf("暂不支持 HLS 加密方式 %s（仅支持 AES-128）", method)
		}
		if formatMatch := keyFormatRE.FindStringSubmatch(trimmed); len(formatMatch) == 2 &&
			!strings.EqualFold(strings.TrimSpace(formatMatch[1]), "identity") {
			return nil, fmt.Errorf("暂不支持 HLS KEYFORMAT %s", formatMatch[1])
		}
		uriMatch := mapURIRE.FindStringSubmatch(trimmed)
		if len(uriMatch) != 2 {
			return nil, fmt.Errorf("AES-128 密钥缺少 URI：%s", trimmed)
		}
		resolved, err := resolveURL(base, uriMatch[1])
		if err != nil {
			return nil, err
		}
		name, exists := keyNames[resolved]
		if !exists {
			name = fmt.Sprintf("key%03d.bin", keyIndex)
			keyNames[resolved] = name
			segments = append(segments, segment{url: resolved, name: name, path: filepath.Join(a.segmentDir, name)})
			keyIndex++
		}
		lines[i] = mapURIRE.ReplaceAllString(trimmed, fmt.Sprintf(`URI="segments/%s"`, name))
	}
	if keyIndex > 0 {
		fmt.Fprintf(consoleOut, "检测到 AES-128 加密 HLS：%d 个密钥，将在本地解密\n", keyIndex)
	}

	mapIndex := 0
	var mapRangeEnd int64
	haveMapRangeEnd := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), "#EXT-X-MAP:") {
			continue
		}
		attributes := parseHLSAttributes(trimmed)
		mapURI := attributes["URI"]
		if mapURI == "" {
			return nil, fmt.Errorf("无法解析 fMP4 初始化段：%s", trimmed)
		}
		resolved, err := resolveURL(base, mapURI)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("init%03d.mp4", mapIndex)
		initSegment := segment{url: resolved, name: name, path: filepath.Join(a.segmentDir, name)}
		if rawRange, ok := attributes["BYTERANGE"]; ok {
			start, length, err := parseByteRange(rawRange, mapRangeEnd, haveMapRangeEnd)
			if err != nil {
				return nil, fmt.Errorf("fMP4 初始化段 %s：%w", mapURI, err)
			}
			initSegment.rangeStart = start
			initSegment.rangeLength = length
			mapRangeEnd = start + length
			haveMapRangeEnd = true
		} else {
			haveMapRangeEnd = false
		}
		segments = append(segments, initSegment)
		lines[i] = fmt.Sprintf(`#EXT-X-MAP:URI="segments/%s"`, name)
		mapIndex++
	}
	if isFMP4 {
		fmt.Fprintf(consoleOut, "检测到 fMP4：%d 个初始化段，%d 个媒体分片\n", mapIndex, len(indices))
	}

	mediaIndex := 0
	mediaEncrypted := false
	var pendingRangeStart, pendingRangeLength int64
	pendingRangeLine := -1
	var mediaRangeEnd int64
	haveMediaRangeEnd := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "#EXT-X-KEY:") {
			if methodMatch := methodRE.FindStringSubmatch(trimmed); len(methodMatch) == 2 {
				mediaEncrypted = !strings.EqualFold(strings.TrimSpace(methodMatch[1]), "NONE")
			}
			continue
		}
		if strings.HasPrefix(strings.ToUpper(trimmed), "#EXT-X-BYTERANGE:") {
			rawRange := strings.TrimSpace(trimmed[strings.IndexByte(trimmed, ':')+1:])
			start, length, err := parseByteRange(rawRange, mediaRangeEnd, haveMediaRangeEnd)
			if err != nil {
				return nil, err
			}
			pendingRangeStart = start
			pendingRangeLength = length
			pendingRangeLine = i
			continue
		}
		if !indexSet[i] {
			continue
		}
		resolved, err := resolveURL(base, trimmed)
		if err != nil {
			return nil, err
		}
		ext := ".ts"
		if isFMP4 {
			ext = ".m4s"
		}
		name := fmt.Sprintf("seg%06d%s", mediaIndex, ext)
		mediaSegment := segment{url: resolved, name: name, path: filepath.Join(a.segmentDir, name), encrypted: mediaEncrypted}
		if pendingRangeLine >= 0 {
			mediaSegment.rangeStart = pendingRangeStart
			mediaSegment.rangeLength = pendingRangeLength
			mediaRangeEnd = pendingRangeStart + pendingRangeLength
			haveMediaRangeEnd = true
			lines[pendingRangeLine] = ""
			pendingRangeLine = -1
			pendingRangeLength = 0
		} else {
			haveMediaRangeEnd = false
		}
		segments = append(segments, mediaSegment)
		lines[i] = filepath.ToSlash(filepath.Join("segments", name))
		mediaIndex++
	}
	if pendingRangeLine >= 0 {
		return nil, errors.New("EXT-X-BYTERANGE 后缺少媒体 URI")
	}
	if err := os.WriteFile(a.localPlaylist, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return nil, err
	}
	return segments, nil
}

func validateHLSKeys(segments []segment) error {
	for _, s := range segments {
		if !strings.HasPrefix(s.name, "key") || !strings.EqualFold(filepath.Ext(s.name), ".bin") {
			continue
		}
		info, err := os.Stat(s.path)
		if err != nil {
			return fmt.Errorf("读取 AES-128 密钥 %s 失败：%w", s.name, err)
		}
		if info.Size() != 16 {
			return fmt.Errorf("AES-128 密钥 %s 长度应为 16 字节，实际为 %d 字节", s.name, info.Size())
		}
	}
	return nil
}

func normalizeTSSegments(segments []segment) (int, error) {
	normalized := 0
	for _, s := range segments {
		if s.encrypted || !strings.EqualFold(filepath.Ext(s.path), ".ts") {
			continue
		}
		data, err := os.ReadFile(s.path)
		if err != nil {
			return normalized, fmt.Errorf("读取分片 %s 失败：%w", s.name, err)
		}
		offset := findMPEGTSOffset(data)
		if offset <= 0 {
			continue
		}
		info, err := os.Stat(s.path)
		if err != nil {
			return normalized, err
		}
		if err := os.WriteFile(s.path, data[offset:], info.Mode()); err != nil {
			return normalized, fmt.Errorf("处理图片包装分片 %s 失败：%w", s.name, err)
		}
		normalized++
	}
	return normalized, nil
}

func findMPEGTSOffset(data []byte) int {
	const (
		packetSize = 188
		scanLimit  = 64 * 1024
	)
	last := len(data) - 2*packetSize - 1
	if last < 0 {
		return -1
	}
	if last > scanLimit {
		last = scanLimit
	}
	for offset := 0; offset <= last; offset++ {
		if data[offset] == 0x47 && data[offset+packetSize] == 0x47 && data[offset+2*packetSize] == 0x47 {
			return offset
		}
	}
	return -1
}

func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func safeName(s string) string {
	s = strings.TrimSpace(s)
	invalid := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	s = invalid.ReplaceAllString(s, "_")
	s = strings.TrimRight(s, ". ")
	if len([]rune(s)) > 120 {
		s = string([]rune(s)[:120])
	}
	if s == "" {
		return "video_" + time.Now().Format("20060102_150405")
	}
	return s
}

func playlistBatchOutputTemplate(title, playlistID string) (string, string) {
	folder := safeName(title)
	runes := []rune(folder)
	if len(runes) > 48 {
		folder = strings.TrimRight(string(runes[:48]), ". ")
	}
	playlistID = strings.TrimSpace(playlistID)
	if playlistID != "" && !strings.Contains(strings.ToLower(folder), strings.ToLower(playlistID)) {
		folder += " [" + safeName(playlistID) + "]"
	}
	escapedFolder := strings.ReplaceAll(folder, "%", "%%")
	template := filepath.ToSlash(filepath.Join(escapedFolder, "%(playlist_index)03d - %(title)s [%(id)s].%(ext)s"))
	return folder, template
}

func uniqueOutput(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return filepath.Join(dir, base+"_"+time.Now().Format("20060102_150405")+ext)
}

func keepWorkError(workDir string, err error) error {
	return fmt.Errorf("%w\n工作目录已保留：%s", err, workDir)
}
