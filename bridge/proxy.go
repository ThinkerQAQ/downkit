package downkit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var proxyConnectivityURLs = [...]string{
	"https://www.google.com/generate_204",
	"https://cp.cloudflare.com/generate_204",
}

type proxyConnectivityResult struct {
	target  string
	latency time.Duration
	err     error
}

// normalizeProxyAddress accepts the legacy full proxy value as well as the
// host/port fields exposed by the proxy tool. DownKit currently supports HTTP
// proxies; HTTPS destinations are carried through them with CONNECT.
func normalizeProxyAddress(raw, host string, port int) (string, string, int, error) {
	raw = strings.TrimSpace(raw)
	host = strings.TrimSpace(host)
	if host == "" && port == 0 && raw == "" {
		return "", "", 0, nil
	}
	if host == "" && port == 0 {
		candidate := raw
		if !strings.Contains(candidate, "://") {
			candidate = "http://" + candidate
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.Port() == "" || parsed.User != nil || parsed.Path != "" {
			return "", "", 0, fmt.Errorf("代理地址 %q 无效；请分别填写主机和端口", raw)
		}
		parsedPort, err := strconv.Atoi(parsed.Port())
		if err != nil {
			return "", "", 0, errors.New("代理端口无效")
		}
		host, port = parsed.Hostname(), parsedPort
	}
	if host == "" || port == 0 {
		return "", "", 0, errors.New("代理主机和端口必须同时填写")
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#@ \t\r\n") {
		return "", "", 0, errors.New("代理主机只填写域名或 IP，不要包含协议、路径或端口")
	}
	if port < 1 || port > 65535 {
		return "", "", 0, errors.New("代理端口必须在 1 到 65535 之间")
	}
	address := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	return address, host, port, nil
}

func proxyHTTPTransport(proxy string) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return transport, nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || proxyURL.Scheme != "http" || proxyURL.Hostname() == "" || proxyURL.Port() == "" {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("无效代理 %q", proxy)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return transport, nil
}

func probeProxyConnectivity(ctx context.Context, proxy, target string) (time.Duration, error) {
	transport, err := proxyHTTPTransport(proxy)
	if err != nil {
		return 0, err
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", "DownKit/"+bridgeVersion)
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return latency, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusInternalServerError {
		return latency, fmt.Errorf("连通性检测返回 HTTP %d", response.StatusCode)
	}
	return latency, nil
}

func probeAnyProxyConnectivity(ctx context.Context, proxy string, targets []string) (time.Duration, string, error) {
	if len(targets) == 0 {
		return 0, "", errors.New("没有配置代理连通性检测端点")
	}

	probeContext, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan proxyConnectivityResult, len(targets))
	started := time.Now()
	for _, target := range targets {
		target := target
		go func() {
			latency, err := probeProxyConnectivity(probeContext, proxy, target)
			results <- proxyConnectivityResult{target: target, latency: latency, err: err}
		}()
	}

	failures := make([]string, 0, len(targets))
	for range targets {
		result := <-results
		if result.err == nil {
			fmt.Fprintf(consoleErr,
				"timestamp=%q severity=%q node=%q operation=%q status=%q target=%q durationMs=%d\n",
				time.Now().Format(time.RFC3339Nano), "info", "network-proxy", "connectivity-probe",
				"success", logURLSummary(result.target), result.latency.Milliseconds())
			return result.latency, result.target, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", logURLSummary(result.target), result.err))
	}

	err := fmt.Errorf("全部代理检测端点均失败：%s", strings.Join(failures, "；"))
	fmt.Fprintf(consoleErr,
		"timestamp=%q severity=%q node=%q operation=%q status=%q targetCount=%d durationMs=%d error=%q\n",
		time.Now().Format(time.RFC3339Nano), "error", "network-proxy", "connectivity-probe",
		"failed", len(targets), time.Since(started).Milliseconds(), err.Error())
	return time.Since(started), "", err
}
