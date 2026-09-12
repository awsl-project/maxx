package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProxyConnectivityResult struct {
	OK         bool   `json:"ok"`
	OutboundIP string `json:"outboundIP,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

func TestProxyConnectivity(ctx context.Context, rawProxy string) ProxyConnectivityResult {
	started := time.Now()
	result := ProxyConnectivityResult{OK: false}
	resolvedProxy, err := resolveProviderProxyURL(rawProxy)
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = err.Error()
		return result
	}
	proxyURL, err := url.Parse(resolvedProxy)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = "invalid provider proxy URL"
		return result
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org?format=json", nil)
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = err.Error()
		return result
	}
	resp, err := client.Do(req)
	if err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = sanitizeProxyCheckError(err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = fmt.Sprintf("probe returned HTTP %d", resp.StatusCode)
		return result
	}
	var body struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		result.DurationMS = time.Since(started).Milliseconds()
		result.Error = "probe returned an invalid response"
		return result
	}
	result.DurationMS = time.Since(started).Milliseconds()
	result.OK = strings.TrimSpace(body.IP) != ""
	result.OutboundIP = strings.TrimSpace(body.IP)
	if !result.OK {
		result.Error = "probe returned an empty outbound IP"
	}
	return result
}

func sanitizeProxyCheckError(err error) string {
	message := err.Error()
	if len(message) > 300 {
		message = message[:300] + "..."
	}
	return message
}
