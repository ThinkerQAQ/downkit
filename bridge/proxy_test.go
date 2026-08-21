package downkit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeProxyAddressMigratesLegacyHostPort(t *testing.T) {
	address, host, port, err := normalizeProxyAddress("127.0.0.1:11111", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if address != "http://127.0.0.1:11111" || host != "127.0.0.1" || port != 11111 {
		t.Fatalf("unexpected normalized proxy: %q %q %d", address, host, port)
	}
}

func TestNormalizeProxyAddressRequiresHostAndPort(t *testing.T) {
	if _, _, _, err := normalizeProxyAddress("", "127.0.0.1", 0); err == nil {
		t.Fatal("expected incomplete proxy error")
	}
	if _, _, _, err := normalizeProxyAddress("", "http://127.0.0.1", 11111); err == nil {
		t.Fatal("expected host-only validation error")
	}
}

func TestNormalizeBridgeConfigPublishesCanonicalProxy(t *testing.T) {
	config := defaultBridgeConfig()
	*config.ProxyEnabled = true
	config.ProxyHost = "127.0.0.1"
	config.ProxyPort = 11111
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Proxy != "http://127.0.0.1:11111" {
		t.Fatalf("proxy = %q", normalized.Proxy)
	}
}

func TestDisabledProxyKeepsEndpointButUsesDirectConnection(t *testing.T) {
	config := defaultBridgeConfig()
	config.ProxyHost = "127.0.0.1"
	config.ProxyPort = 11111
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Proxy != "" || normalized.ProxyHost != "127.0.0.1" || normalized.ProxyPort != 11111 {
		t.Fatalf("disabled proxy was not preserved: %#v", normalized)
	}
	if proxyEnabled(normalized) {
		t.Fatal("proxy should remain disabled")
	}
}

func TestLegacyConfiguredProxyDefaultsToEnabled(t *testing.T) {
	config := defaultBridgeConfig()
	config.ProxyEnabled = nil
	config.ProxyHost = "127.0.0.1"
	config.ProxyPort = 11111
	normalized, err := normalizeBridgeConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if !proxyEnabled(normalized) || normalized.Proxy != "http://127.0.0.1:11111" {
		t.Fatalf("legacy proxy was not enabled: %#v", normalized)
	}
}

func TestEnabledProxyRequiresEndpoint(t *testing.T) {
	config := defaultBridgeConfig()
	*config.ProxyEnabled = true
	if _, err := normalizeBridgeConfig(config); err == nil {
		t.Fatal("expected enabled proxy without endpoint to fail")
	}
}

func TestProbeProxyConnectivityUsesConfiguredProxy(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		response, err := http.Get(r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.WriteHeader(response.StatusCode)
	}))
	defer proxy.Close()
	if _, err := probeProxyConnectivity(context.Background(), proxy.URL, target.URL); err != nil {
		t.Fatal(err)
	}
	if proxyHits != 1 || targetHits != 1 {
		t.Fatalf("probe did not traverse proxy: proxy=%d target=%d", proxyHits, targetHits)
	}
}
