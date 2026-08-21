package downkit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchHTTPHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "test-agent" || r.Referer() != "https://page.example/video" || r.Header.Get("Origin") != "https://page.example" || r.Header.Get("Cookie") != "session=secret" {
			t.Errorf("unexpected headers: UA=%q Referer=%q Origin=%q Cookie=%q", r.UserAgent(), r.Referer(), r.Header.Get("Origin"), r.Header.Get("Cookie"))
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer server.Close()
	a := app{opts: options{
		sourceURL: server.URL, userAgent: "test-agent", referer: "https://page.example/video", origin: "https://page.example",
		mediaCookies: []browserCookie{{Name: "session", Value: "secret", Domain: "127.0.0.1", HostOnly: true, Path: "/"}},
	}}
	body, status, err := a.fetchHTTP(server.URL)
	if err != nil || status != http.StatusOK || !strings.HasPrefix(string(body), "#EXTM3U") {
		t.Fatalf("fetchHTTP failed: status=%d body=%q err=%v", status, body, err)
	}
}

func TestHLSRequestsShareTaskCookies(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/master.m3u8":
			if cookie, err := r.Cookie("browser"); err != nil || cookie.Value != "seed" {
				http.Error(w, "missing browser session", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "rotated", Value: "ready", Path: "/"})
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nvariant.m3u8\n"))
		case "/variant.m3u8":
			if cookie, err := r.Cookie("rotated"); err != nil || cookie.Value != "ready" {
				http.Error(w, "playlist session was not preserved", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:1,\nsegment.ts\n#EXT-X-ENDLIST\n"))
		case "/segment.ts":
			if cookie, err := r.Cookie("rotated"); err != nil || cookie.Value != "ready" {
				http.Error(w, "segment session was not preserved", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte("segment"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	a := &app{opts: options{
		sourceURL:    server.URL + "/master.m3u8",
		concurrent:   1,
		mediaCookies: []browserCookie{{Name: "browser", Value: "seed", Domain: "127.0.0.1", HostOnly: true, Path: "/"}},
	}}
	if _, status, err := a.fetchHTTP(server.URL + "/master.m3u8"); err != nil || status != http.StatusOK {
		t.Fatalf("master request failed: status=%d err=%v", status, err)
	}
	if _, status, err := a.fetchHTTP(server.URL + "/variant.m3u8"); err != nil || status != http.StatusOK {
		t.Fatalf("variant request did not share session: status=%d err=%v", status, err)
	}
	d, err := newGoDownloader(a, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	item := segment{url: server.URL + "/segment.ts", name: "segment", path: filepath.Join(dir, "segment.ts")}
	if err := d.downloadSegment(item); err != nil {
		t.Fatalf("segment request did not share session: %v", err)
	}
	assertFileContent(t, item.path, []byte("segment"))
}

func TestCapturedCredentialsStayOnOriginalHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("sensitive headers leaked cross-host: Cookie=%q Authorization=%q", r.Header.Get("Cookie"), r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer server.Close()
	a := app{opts: options{
		sourceURL: "https://original.test/master.m3u8",
		requestHeaders: map[string]string{
			"Authorization": "Bearer secret",
		},
	}}
	if _, _, err := a.fetchHTTP(server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestParseRequestHeaders(t *testing.T) {
	headers, err := parseRequestHeaders(`{"cookie":"a=b","authorization":"Bearer test","host":"blocked","accept-encoding":"br"}`)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Cookie"] != "" || headers["Authorization"] != "Bearer test" {
		t.Fatalf("missing forwarded headers: %#v", headers)
	}
	if _, ok := headers["Host"]; ok {
		t.Fatalf("Host must not be forwarded: %#v", headers)
	}
	if _, ok := headers["Accept-Encoding"]; ok {
		t.Fatalf("Accept-Encoding must not be forwarded: %#v", headers)
	}
}

func TestIsNativeMessagingInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "chromium origin", args: []string{"chrome-extension://hfjpenjlamneepmigemmmiibebpokmik/"}, want: true},
		{name: "explicit debug flag", args: []string{"--native-host"}, want: true},
		{name: "bridge", args: []string{"--bridge"}, want: false},
		{name: "download URL", args: []string{"downkit://download?url=test"}, want: false},
		{name: "no arguments", args: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isNativeMessagingInvocation(test.args); got != test.want {
				t.Fatalf("isNativeMessagingInvocation(%q) = %v; want %v", test.args, got, test.want)
			}
		})
	}
}

func TestLogURLSummaryRedactsSensitiveParts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "signed media URL", raw: "https://user:pass@cdn.example/video/secret.m3u8?token=abc#fragment", want: "https://cdn.example/<redacted>"},
		{name: "proxy credentials", raw: "http://proxy-user:proxy-pass@127.0.0.1:8080", want: "http://127.0.0.1:8080/<redacted>"},
		{name: "empty optional header", raw: "", want: ""},
		{name: "invalid", raw: "://bad", want: "<invalid-url>"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := logURLSummary(test.raw); got != test.want {
				t.Fatalf("logURLSummary(%q) = %q, want %q", test.raw, got, test.want)
			}
		})
	}
}

func TestBridgeConfigProvidesDefaultQuality(t *testing.T) {
	config := defaultBridgeConfig()
	config.Quality = "720"
	opts, err := optionsFromBridgeTask(bridgeTask{
		URL:   "https://media.test/master.m3u8",
		Title: "test",
	}, config)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.qualitySet || opts.quality != 720 {
		t.Fatalf("default quality not applied: set=%v quality=%d", opts.qualitySet, opts.quality)
	}

	opts, err = optionsFromBridgeTask(bridgeTask{
		URL:     "https://media.test/master.m3u8",
		Title:   "test",
		Quality: "best",
	}, config)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.qualitySet || opts.quality != 0 {
		t.Fatalf("task quality did not override config: set=%v quality=%d", opts.qualitySet, opts.quality)
	}
}

func TestProtocolRejectsCookieRequestHeader(t *testing.T) {
	rawHeaders := `{"cookie":"session=secret","authorization":"Bearer test"}`
	raw := "downkit://download?url=" + url.QueryEscape("https://cdn.test/master.m3u8") + "&headers=" + url.QueryEscape(rawHeaders)
	var opts options
	if err := applyProtocolURI(&opts, raw); err != nil {
		t.Fatal(err)
	}
	if opts.requestHeaders["Cookie"] != "" || opts.requestHeaders["Authorization"] != "Bearer test" {
		t.Fatalf("protocol headers lost: %#v", opts.requestHeaders)
	}
}

func TestBridgeTaskCarriesStructuredCookiesOutsideHeaders(t *testing.T) {
	opts, err := optionsFromBridgeTask(bridgeTask{
		URL: "https://www.youtube.com/watch?v=test", Title: "test", ResolvePage: true,
		MediaHeaders: map[string]string{"Cookie": "must=drop", "Authorization": "Bearer test"},
		PageHeaders:  map[string]string{"Cookie": "must=drop"},
		PageCookies:  []browserCookie{{Name: "SID", Value: "secret", Domain: ".youtube.com", Path: "/", Secure: true, HTTPOnly: true}},
	}, defaultBridgeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if opts.requestHeaders["Cookie"] != "" || opts.pageRequestHeaders["Cookie"] != "" {
		t.Fatalf("cookie leaked into headers: media=%#v page=%#v", opts.requestHeaders, opts.pageRequestHeaders)
	}
	if len(opts.pageCookies) != 1 || opts.pageCookies[0].Domain != ".youtube.com" || !opts.pageCookies[0].HTTPOnly {
		t.Fatalf("structured cookie attributes lost: %#v", opts.pageCookies)
	}
}

func TestParseFFmpegVersion(t *testing.T) {
	tests := []struct {
		line        string
		major       int
		minor       int
		development bool
	}{
		{"ffmpeg version 4.4.8 Copyright", 4, 4, false},
		{"ffmpeg version n8.1.2 Copyright", 8, 1, false},
		{"ffmpeg version 2026-05-11-git-17bc88e67f Copyright", 0, 0, true},
		{"ffmpeg version N-117000-gabcdef Copyright", 0, 0, true},
	}
	for _, test := range tests {
		got, err := parseFFmpegVersion(test.line)
		if err != nil || got.major != test.major || got.minor != test.minor || got.development != test.development {
			t.Fatalf("parseFFmpegVersion(%q) = %+v, %v", test.line, got, err)
		}
	}
	oldDev, err := parseFFmpegVersion("ffmpeg version N-94813-g85386c36e3 Copyright")
	if err != nil || checkFFmpegMinimum(oldDev) == nil {
		t.Fatalf("expected old development build rejection: %+v, %v", oldDev, err)
	}
	oldStable, err := parseFFmpegVersion("ffmpeg version 4.3.9 Copyright")
	if err != nil || checkFFmpegMinimum(oldStable) == nil {
		t.Fatalf("expected FFmpeg 4.3 rejection: %+v, %v", oldStable, err)
	}
}

func TestToolExecutableNamesPreferSlimFFmpeg(t *testing.T) {
	names := toolExecutableNames("ffmpeg")
	if len(names) != 2 || !strings.HasPrefix(names[0], "ffmpeg-slim") || !strings.HasPrefix(names[1], "ffmpeg") {
		t.Fatalf("unexpected FFmpeg candidates: %#v", names)
	}
	ytDLP := toolExecutableNames("yt-dlp")
	if len(ytDLP) != 1 || !strings.HasPrefix(ytDLP[0], "yt-dlp") {
		t.Fatalf("unexpected yt-dlp candidates: %#v", ytDLP)
	}
}

func TestNormalizePlaylistMode(t *testing.T) {
	for input, want := range map[string]string{
		"": "ask", "ASK": "ask", " single ": "single", "all": "all",
	} {
		got, err := normalizePlaylistMode(input)
		if err != nil || got != want {
			t.Fatalf("normalizePlaylistMode(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizePlaylistMode("everything"); err == nil {
		t.Fatal("expected invalid playlist mode error")
	}
}

func TestChoosePlaylistMode(t *testing.T) {
	var output bytes.Buffer
	if got := choosePlaylistMode(strings.NewReader("2\n"), &output, 28); got != "all" {
		t.Fatalf("expected all, got %q", got)
	}
	if !strings.Contains(output.String(), "28") {
		t.Fatalf("prompt does not contain playlist count: %q", output.String())
	}
	output.Reset()
	if got := choosePlaylistMode(strings.NewReader("\n"), &output, 28); got != "single" {
		t.Fatalf("expected default single, got %q", got)
	}
}

func TestPlaylistBatchOutputTemplateKeepsPathsShort(t *testing.T) {
	longTitle := strings.Repeat("很长的课程标题", 20)
	folder, template := playlistBatchOutputTemplate(longTitle, "course-42")
	if len([]rune(folder)) > 64 {
		t.Fatalf("batch folder is too long: %d runes, %q", len([]rune(folder)), folder)
	}
	if !strings.Contains(folder, "course-42") {
		t.Fatalf("batch folder does not contain playlist id: %q", folder)
	}
	if !strings.Contains(template, "%(playlist_index)03d") || !strings.Contains(template, "%(title)s") {
		t.Fatalf("unexpected output template: %q", template)
	}
}

func TestPlaylistInfoBuildsOrderedJobFiles(t *testing.T) {
	info := ytDLPPlaylistInfo{Entries: []json.RawMessage{
		json.RawMessage(`{"id":"ep-1","title":"第一集","playlist_index":1}`),
		json.RawMessage(`{"id":"ep-2","title":"第二集"}`),
	}}
	files := info.jobFiles()
	if len(files) != 2 || files[0].Index != 1 || files[0].Title != "第一集" || files[1].Index != 2 || files[1].ID != "ep-2" {
		t.Fatalf("unexpected job files: %#v", files)
	}
}

func TestPageForSeparatedMedia(t *testing.T) {
	page, ok := pageForSeparatedMedia(
		"https://video.cdn.test/media/video.m4s?token=1",
		"https://page.test/watch/42?part=3#player",
		false,
	)
	if !ok || page != "https://page.test/watch/42?part=3" {
		t.Fatalf("unexpected result: page=%q ok=%v", page, ok)
	}
	page, ok = pageForSeparatedMedia("https://page.test/watch/42#player", "", true)
	if !ok || page != "https://page.test/watch/42" {
		t.Fatalf("forced page resolution failed: page=%q ok=%v", page, ok)
	}
	page, ok = pageForSeparatedMedia("https://media.test/manifest.mpd#dash", "", false)
	if !ok || page != "https://media.test/manifest.mpd" {
		t.Fatalf("direct DASH resolver failed: page=%q ok=%v", page, ok)
	}

	for _, test := range []struct {
		source  string
		referer string
	}{
		{"https://media.test/video.mp4", "https://page.test/watch"},
		{"https://media.test/video.ts", "https://page.test/watch"},
		{"not-a-url", "https://page.test/watch"},
	} {
		if _, ok := pageForSeparatedMedia(test.source, test.referer, false); ok {
			t.Fatalf("unexpected match: %+v", test)
		}
	}
}

func TestWriteTaskCookieFilePreservesStructuredAttributes(t *testing.T) {
	dir := t.TempDir()
	path, err := writeTaskCookieFile(dir, "task-cookies.txt", []browserCookie{
		{Name: "session", Value: "secret", Domain: ".youtube.com", Path: "/", Secure: true, HTTPOnly: true, ExpirationDate: 1999999999},
		{Name: "host", Value: "only", Domain: "www.youtube.com", HostOnly: true, Path: "/watch", Secure: true, Session: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t1999999999\tsession\tsecret") ||
		!strings.Contains(text, "www.youtube.com\tFALSE\t/watch\tTRUE\t0\thost\tonly") {
		t.Fatalf("unexpected task cookie file: %q", text)
	}
}

func TestPrepareAES128Playlist(t *testing.T) {
	workDir := t.TempDir()
	segmentDir := filepath.Join(workDir, "segments")
	if err := os.MkdirAll(segmentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := app{
		workDir:       workDir,
		segmentDir:    segmentDir,
		localPlaylist: filepath.Join(workDir, "local.m3u8"),
	}
	playlist := `#EXTM3U
#EXT-X-MEDIA-SEQUENCE:7
#EXT-X-KEY:METHOD=AES-128,URI="crypt.key",IV=0x00112233445566778899aabbccddeeff
#EXTINF:5,
seg0.ts
#EXTINF:5,
seg1.ts
#EXT-X-ENDLIST`
	segments, err := a.prepareMediaPlaylist(playlist, "https://media.example/path/index.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 {
		t.Fatalf("expected key plus two media segments, got %d", len(segments))
	}
	if segments[0].url != "https://media.example/path/crypt.key" || segments[0].name != "key000.bin" {
		t.Fatalf("unexpected key segment: %+v", segments[0])
	}
	if !segments[1].encrypted || !segments[2].encrypted {
		t.Fatalf("AES-128 media segments were not marked encrypted: %+v", segments)
	}
	local, err := os.ReadFile(a.localPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	text := string(local)
	if !strings.Contains(text, `#EXT-X-KEY:METHOD=AES-128,URI="segments/key000.bin",IV=0x00112233445566778899aabbccddeeff`) {
		t.Fatalf("key line was not rewritten correctly:\n%s", text)
	}
	if !strings.Contains(text, "segments/seg000000.ts") || !strings.Contains(text, "segments/seg000001.ts") {
		t.Fatalf("media segments were not rewritten correctly:\n%s", text)
	}
}

func TestPrepareByteRangePlaylist(t *testing.T) {
	workDir := t.TempDir()
	a := app{
		workDir:       workDir,
		segmentDir:    filepath.Join(workDir, "segments"),
		localPlaylist: filepath.Join(workDir, "local.m3u8"),
	}
	playlist := `#EXTM3U
#EXT-X-MAP:URI="stream.mp4",BYTERANGE="100@0"
#EXTINF:5,
#EXT-X-BYTERANGE:200@100
stream.mp4
#EXTINF:5,
#EXT-X-BYTERANGE:300
stream.mp4
#EXT-X-ENDLIST`
	segments, err := a.prepareMediaPlaylist(playlist, "https://media.example/video/index.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 {
		t.Fatalf("expected init plus two media ranges, got %d", len(segments))
	}
	want := [][2]int64{{0, 100}, {100, 200}, {300, 300}}
	for i, segment := range segments {
		if segment.rangeStart != want[i][0] || segment.rangeLength != want[i][1] {
			t.Fatalf("segment %d range = %d+%d, want %d+%d", i, segment.rangeStart, segment.rangeLength, want[i][0], want[i][1])
		}
	}
	local, err := os.ReadFile(a.localPlaylist)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(local), "BYTERANGE") {
		t.Fatalf("local playlist still contains BYTERANGE:\n%s", local)
	}
}

func TestAudioRenditions(t *testing.T) {
	master := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="main",NAME="中文,立体声",LANGUAGE="zh",DEFAULT=YES,AUTOSELECT=YES,URI="audio/zh.m3u8"
#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=1280x720,AUDIO="main"
video.m3u8`
	renditions, err := audioRenditions(master, "https://media.example/master.m3u8", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(renditions) != 1 || renditions[0].name != "中文,立体声" || !renditions[0].isDefault {
		t.Fatalf("unexpected renditions: %+v", renditions)
	}
	if renditions[0].url != "https://media.example/audio/zh.m3u8" {
		t.Fatalf("unexpected audio URL: %q", renditions[0].url)
	}
	variant, ok, err := selectVariant(master, "https://media.example/master.m3u8", 0, true)
	if err != nil || !ok || variant.audioGroup != "main" {
		t.Fatalf("variant audio group not parsed: %+v, ok=%v, err=%v", variant, ok, err)
	}
}

func TestRejectUnsupportedHLSKeyMethod(t *testing.T) {
	workDir := t.TempDir()
	a := app{
		workDir:       workDir,
		segmentDir:    filepath.Join(workDir, "segments"),
		localPlaylist: filepath.Join(workDir, "local.m3u8"),
	}
	_, err := a.prepareMediaPlaylist("#EXTM3U\n#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"key.bin\"\nseg.ts", "https://example.com/a.m3u8")
	if err == nil || !strings.Contains(err.Error(), "SAMPLE-AES") {
		t.Fatalf("expected SAMPLE-AES rejection, got %v", err)
	}
}

func TestValidateHLSKeys(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "key000.bin")
	if err := os.WriteFile(validPath, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateHLSKeys([]segment{{name: "key000.bin", path: validPath}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(validPath, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateHLSKeys([]segment{{name: "key000.bin", path: validPath}}); err == nil {
		t.Fatal("expected invalid key length error")
	}
}
