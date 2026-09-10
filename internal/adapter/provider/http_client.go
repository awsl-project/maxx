package provider

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	golangproxy "golang.org/x/net/proxy"
)

// NewHTTPClient builds an upstream HTTP client for provider adapters. A provider
// proxyURL is explicit and never falls back to direct on parse/setup errors.
// When proxyURL is empty, inheritEnvProxy controls whether HTTP(S)_PROXY from
// the process environment remains active for legacy native adapters.
func NewHTTPClient(p *domain.Provider, timeout time.Duration, inheritEnvProxy bool) (*http.Client, error) {
	transport, err := NewHTTPTransport(p, inheritEnvProxy)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

func NewHTTPTransport(p *domain.Provider, inheritEnvProxy bool) (*http.Transport, error) {
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 60 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	rawProxy := providerProxyURL(p)
	if rawProxy == "" {
		if inheritEnvProxy {
			transport.Proxy = http.ProxyFromEnvironment
		}
		return transport, nil
	}

	proxyURL, err := url.Parse(rawProxy)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		return nil, fmt.Errorf("invalid provider proxy URL")
	}

	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		proxyForDialer := *proxyURL
		proxyForDialer.Scheme = "socks5"
		dialer, err := golangproxy.FromURL(&proxyForDialer, golangproxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("invalid provider SOCKS proxy")
		}
		ctxDialer, ok := dialer.(golangproxy.ContextDialer)
		if ok {
			transport.DialContext = ctxDialer.DialContext
		} else {
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				type result struct {
					conn net.Conn
					err  error
				}
				ch := make(chan result, 1)
				go func() { conn, err := dialer.Dial(network, addr); ch <- result{conn: conn, err: err} }()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case r := <-ch:
					return r.conn, r.err
				}
			}
		}
	default:
		return nil, fmt.Errorf("unsupported provider proxy scheme")
	}
	return transport, nil
}

func providerProxyURL(p *domain.Provider) string {
	if p == nil || p.Config == nil {
		return ""
	}
	return strings.TrimSpace(p.Config.ProxyURL)
}
