package downkit

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// mediaHTTPSession is scoped to one download task. It lets master playlists,
// media playlists, keys and segments observe Set-Cookie updates from earlier
// responses without sharing browser credentials with other jobs.
type mediaHTTPSession struct {
	jar http.CookieJar
}

func newMediaHTTPSession(opts options) *mediaHTTPSession {
	jar, _ := cookiejar.New(nil)
	session := &mediaHTTPSession{jar: jar}
	session.seed(opts.mediaCookies)
	return session
}

func (s *mediaHTTPSession) seed(values []browserCookie) {
	byOrigin := make(map[string][]*http.Cookie)
	for _, value := range sanitizeBrowserCookies(values) {
		host := strings.TrimPrefix(value.Domain, ".")
		scheme := "http"
		if value.Secure {
			scheme = "https"
		}
		origin := scheme + "://" + host + value.Path
		expires := time.Time{}
		if !value.Session && value.ExpirationDate > 0 {
			expires = time.Unix(int64(value.ExpirationDate), 0)
		}
		byOrigin[origin] = append(byOrigin[origin], &http.Cookie{
			Name: value.Name, Value: value.Value, Domain: value.Domain, Path: value.Path,
			Secure: value.Secure, HttpOnly: value.HTTPOnly, Expires: expires,
		})
	}
	for rawURL, cookies := range byOrigin {
		if parsed, err := url.Parse(rawURL); err == nil {
			s.jar.SetCookies(parsed, cookies)
		}
	}
}

func (a *app) ensureMediaHTTPSession() *mediaHTTPSession {
	if a.mediaSession == nil {
		a.mediaSession = newMediaHTTPSession(a.opts)
	}
	return a.mediaSession
}

func (a *app) mediaHTTPClient(transport http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		Jar:       a.ensureMediaHTTPSession().jar,
	}
}

func (a *app) applyTaskMediaRequestHeaders(request *http.Request) {
	applyMediaRequestHeaders(request, a.opts)
	// The task cookie was seeded into the jar. Let net/http choose cookies for
	// redirects and descendant URLs and replace rotated values from Set-Cookie.
	request.Header.Del("Cookie")
}

func accessContextSummary(opts options) string {
	states := []string{
		"Cookie=" + presenceLabelCount(len(opts.mediaCookies)),
		"Referer=" + presenceLabel(opts.referer),
		"Origin=" + presenceLabel(opts.origin),
		"User-Agent=" + presenceLabel(opts.userAgent),
	}
	return strings.Join(states, "，")
}

func presenceLabelCount(count int) string {
	if count == 0 {
		return "未捕获"
	}
	return "已携带"
}

func presenceLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "未捕获"
	}
	return "已携带"
}
