package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewHTTPClientUsesExplicitHTTPProxy(t *testing.T) {
	proxyHit := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit <- r.URL.String()
		_, _ = io.WriteString(w, "proxied")
	}))
	defer proxy.Close()

	client, err := NewHTTPClient(&domain.Provider{Config: &domain.ProviderConfig{ProxyURL: proxy.URL}}, time.Second, false)
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.test/probe", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("proxied request failed: %v", err)
	}
	_ = resp.Body.Close()

	select {
	case got := <-proxyHit:
		if got != "http://example.test/probe" {
			t.Fatalf("proxy saw %q, want absolute upstream URL", got)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy was not used")
	}
}

func TestNewHTTPClientRejectsInvalidExplicitProxy(t *testing.T) {
	_, err := NewHTTPClient(&domain.Provider{Config: &domain.ProviderConfig{ProxyURL: "127.0.0.1:7890"}}, time.Second, false)
	if err == nil {
		t.Fatal("invalid explicit proxy must fail instead of falling back direct")
	}
}

func TestNewHTTPClientEmptyProxyDoesNotSetExplicitProxy(t *testing.T) {
	client, err := NewHTTPClient(&domain.Provider{Config: &domain.ProviderConfig{}}, time.Second, false)
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("empty provider proxy should not install explicit proxy")
	}
}
