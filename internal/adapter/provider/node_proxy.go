package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxManagedNodeProxyCacheEntries = 64

var managedNodeProxyCache = struct {
	sync.Mutex
	items  map[string]*managedNodeProxy
	starts map[string]*nodeProxyStart
}{items: map[string]*managedNodeProxy{}, starts: map[string]*nodeProxyStart{}}

type managedNodeProxy struct {
	url    string
	cancel context.CancelFunc
	done   chan struct{}
	config string
}

type nodeProxyStart struct {
	done  chan struct{}
	proxy *managedNodeProxy
	err   error
}

func isNodeProxyScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "vmess", "vless", "trojan", "ss":
		return true
	default:
		return false
	}
}

func resolveProviderProxyURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	scheme := proxyScheme(trimmed)
	if !isNodeProxyScheme(scheme) {
		return trimmed, nil
	}
	return startManagedNodeProxy(trimmed)
}

func proxyScheme(raw string) string {
	idx := strings.Index(raw, "://")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(raw[:idx]))
}

func stripProxyScheme(raw string) string {
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return raw
	}
	return raw[idx+3:]
}

func startManagedNodeProxy(raw string) (string, error) {
	for {
		managedNodeProxyCache.Lock()
		if cached := managedNodeProxyCache.items[raw]; isManagedNodeProxyAlive(cached) {
			url := cached.url
			managedNodeProxyCache.Unlock()
			return url, nil
		}
		delete(managedNodeProxyCache.items, raw)
		if start := managedNodeProxyCache.starts[raw]; start != nil {
			done := start.done
			managedNodeProxyCache.Unlock()
			<-done
			if start.err != nil {
				return "", start.err
			}
			if isManagedNodeProxyAlive(start.proxy) {
				return start.proxy.url, nil
			}
			continue
		}
		if len(managedNodeProxyCache.items)+len(managedNodeProxyCache.starts) >= maxManagedNodeProxyCacheEntries {
			managedNodeProxyCache.Unlock()
			return "", fmt.Errorf("too many active managed node proxies")
		}
		start := &nodeProxyStart{done: make(chan struct{})}
		managedNodeProxyCache.starts[raw] = start
		managedNodeProxyCache.Unlock()

		proxy, err := createManagedNodeProxy(raw)

		managedNodeProxyCache.Lock()
		start.proxy = proxy
		start.err = err
		if err == nil {
			managedNodeProxyCache.items[raw] = proxy
		}
		delete(managedNodeProxyCache.starts, raw)
		close(start.done)
		managedNodeProxyCache.Unlock()
		if err != nil {
			return "", err
		}
		return proxy.url, nil
	}
}

func isManagedNodeProxyAlive(proxy *managedNodeProxy) bool {
	if proxy == nil {
		return false
	}
	select {
	case <-proxy.done:
		return false
	default:
		return true
	}
}

func createManagedNodeProxy(raw string) (*managedNodeProxy, error) {
	bin, err := findSingBoxBinary()
	if err != nil {
		return nil, err
	}
	outbound, err := buildSingBoxOutbound(raw)
	if err != nil {
		return nil, err
	}
	port, err := reserveLocalPort()
	if err != nil {
		return nil, fmt.Errorf("reserve node proxy port: %w", err)
	}
	configPath, err := writeSingBoxConfig(port, outbound)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "run", "-c", configPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.Remove(configPath)
		return nil, fmt.Errorf("start sing-box node proxy: %w", err)
	}
	done := make(chan struct{})
	proxy := &managedNodeProxy{url: fmt.Sprintf("socks5h://127.0.0.1:%d", port), cancel: cancel, done: done, config: configPath}
	go func() {
		_ = cmd.Wait()
		_ = os.Remove(configPath)
		managedNodeProxyCache.Lock()
		for key, item := range managedNodeProxyCache.items {
			if item == proxy {
				delete(managedNodeProxyCache.items, key)
			}
		}
		managedNodeProxyCache.Unlock()
		close(done)
	}()
	if err := waitForNodeProxyReady(port, done, 3*time.Second); err != nil {
		cancel()
		<-done
		return nil, err
	}
	return proxy, nil
}

func findSingBoxBinary() (string, error) {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("MAXX_SING_BOX_PATH")); configured != "" {
		candidates = append(candidates, configured)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates, filepath.Join(dir, "sing-box"), filepath.Join(dir, "bin", "sing-box"))
	}
	candidates = append(candidates, "/app/sing-box", "/app/bin/sing-box")
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	if bin, err := exec.LookPath("sing-box"); err == nil {
		return bin, nil
	}
	return "", fmt.Errorf("provider node proxy requires sing-box; set MAXX_SING_BOX_PATH or use a Maxx image that bundles sing-box")
}

func reserveLocalPort() (int, error) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected listener address")
	}
	return addr.Port, nil
}

func waitForNodeProxyReady(port int, done <-chan struct{}, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return fmt.Errorf("sing-box node proxy exited during startup")
		default:
		}
		dialCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
		cancel()
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("sing-box node proxy did not become ready")
}

func writeSingBoxConfig(port int, outbound map[string]any) (string, error) {
	config := map[string]any{
		"log": map[string]any{"disabled": true},
		"inbounds": []map[string]any{{
			"type":        "mixed",
			"tag":         "maxx-node-in",
			"listen":      "127.0.0.1",
			"listen_port": port,
		}},
		"outbounds": []map[string]any{outbound},
	}
	data, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "maxx-node-proxy-*.json")
	if err != nil {
		return "", fmt.Errorf("create sing-box node proxy config: %w", err)
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("secure sing-box node proxy config: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write sing-box node proxy config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close sing-box node proxy config: %w", err)
	}
	return path, nil
}

func buildSingBoxOutbound(raw string) (map[string]any, error) {
	switch proxyScheme(raw) {
	case "vmess":
		return buildVMessOutbound(raw)
	case "vless":
		return buildVLESSOutbound(raw)
	case "trojan":
		return buildTrojanOutbound(raw)
	case "ss":
		return buildShadowsocksOutbound(raw)
	default:
		return nil, fmt.Errorf("unsupported node proxy scheme")
	}
}

func buildVMessOutbound(raw string) (map[string]any, error) {
	payload := stripProxyScheme(raw)
	decoded, err := decodeBase64Loose(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid vmess link")
	}
	var vmess struct {
		Address  string `json:"add"`
		Port     any    `json:"port"`
		UUID     string `json:"id"`
		AlterID  any    `json:"aid"`
		Security string `json:"scy"`
		Network  string `json:"net"`
		Host     string `json:"host"`
		Path     string `json:"path"`
		TLS      string `json:"tls"`
		SNI      string `json:"sni"`
		Name     string `json:"ps"`
	}
	if err := json.Unmarshal(decoded, &vmess); err != nil {
		return nil, fmt.Errorf("invalid vmess link")
	}
	serverPort, err := parseAnyPort(vmess.Port)
	if err != nil || vmess.Address == "" || vmess.UUID == "" {
		return nil, fmt.Errorf("invalid vmess link")
	}
	out := map[string]any{
		"type":        "vmess",
		"tag":         "maxx-node-out",
		"server":      vmess.Address,
		"server_port": serverPort,
		"uuid":        vmess.UUID,
		"security":    firstNonEmpty(vmess.Security, "auto"),
	}
	if aid, ok := parseOptionalInt(vmess.AlterID); ok {
		out["alter_id"] = aid
	}
	applySingBoxTLS(out, strings.EqualFold(vmess.TLS, "tls"), vmess.SNI, "")
	if err := applySingBoxTransport(out, vmess.Network, vmess.Host, vmess.Path, ""); err != nil {
		return nil, err
	}
	return out, nil
}

func buildVLESSOutbound(raw string) (map[string]any, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
		return nil, fmt.Errorf("invalid vless link")
	}
	port, err := parseURLPort(u)
	if err != nil {
		return nil, fmt.Errorf("invalid vless link")
	}
	q := u.Query()
	out := map[string]any{"type": "vless", "tag": "maxx-node-out", "server": u.Hostname(), "server_port": port, "uuid": u.User.Username()}
	if flow := q.Get("flow"); flow != "" {
		out["flow"] = flow
	}
	security := strings.ToLower(strings.TrimSpace(q.Get("security")))
	switch security {
	case "", "none":
	case "tls", "reality":
		applySingBoxTLS(out, true, firstNonEmpty(q.Get("sni"), q.Get("serverName")), security)
	default:
		return nil, fmt.Errorf("unsupported vless security")
	}
	if security == "reality" {
		applySingBoxRealityOptions(out, q.Get("pbk"), q.Get("sid"), q.Get("fp"))
	}
	if err := applySingBoxTransport(out, q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName")); err != nil {
		return nil, err
	}
	return out, nil
}

func buildTrojanOutbound(raw string) (map[string]any, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
		return nil, fmt.Errorf("invalid trojan link")
	}
	port, err := parseURLPort(u)
	if err != nil {
		return nil, fmt.Errorf("invalid trojan link")
	}
	q := u.Query()
	out := map[string]any{"type": "trojan", "tag": "maxx-node-out", "server": u.Hostname(), "server_port": port, "password": u.User.Username()}
	applySingBoxTLS(out, true, firstNonEmpty(q.Get("sni"), q.Get("peer")), "")
	if err := applySingBoxTransport(out, q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName")); err != nil {
		return nil, err
	}
	return out, nil
}

func buildShadowsocksOutbound(raw string) (map[string]any, error) {
	body := stripProxyScheme(raw)
	body = strings.SplitN(body, "#", 2)[0]
	body = strings.SplitN(body, "?", 2)[0]
	if !strings.Contains(body, "@") {
		decoded, err := decodeBase64Loose(body)
		if err != nil {
			return nil, fmt.Errorf("invalid shadowsocks link")
		}
		body = string(decoded)
	} else {
		parts := strings.SplitN(body, "@", 2)
		if decoded, err := decodeBase64Loose(parts[0]); err == nil && strings.Contains(string(decoded), ":") {
			body = string(decoded) + "@" + parts[1]
		}
	}
	u, err := url.Parse("ss://" + body)
	if err != nil || u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
		return nil, fmt.Errorf("invalid shadowsocks link")
	}
	password, _ := u.User.Password()
	if password == "" {
		return nil, fmt.Errorf("invalid shadowsocks link")
	}
	port, err := parseURLPort(u)
	if err != nil {
		return nil, fmt.Errorf("invalid shadowsocks link")
	}
	return map[string]any{"type": "shadowsocks", "tag": "maxx-node-out", "server": u.Hostname(), "server_port": port, "method": u.User.Username(), "password": password}, nil
}

func applySingBoxTLS(out map[string]any, enabled bool, serverName string, security string) {
	if !enabled {
		return
	}
	tls := map[string]any{"enabled": true}
	if serverName != "" {
		tls["server_name"] = serverName
	}
	if security == "reality" {
		tls["reality"] = map[string]any{"enabled": true}
	}
	out["tls"] = tls
}

func applySingBoxRealityOptions(out map[string]any, publicKey, shortID, fingerprint string) {
	tls, ok := out["tls"].(map[string]any)
	if !ok {
		return
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		return
	}
	if publicKey != "" {
		reality["public_key"] = publicKey
	}
	if shortID != "" {
		reality["short_id"] = shortID
	}
	if fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
}

func applySingBoxTransport(out map[string]any, network, host, path, serviceName string) error {
	switch strings.ToLower(strings.TrimSpace(network)) {
	case "", "tcp":
		return nil
	case "ws", "websocket":
		transport := map[string]any{"type": "ws"}
		if path != "" {
			transport["path"] = path
		}
		if host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		out["transport"] = transport
		return nil
	case "grpc":
		transport := map[string]any{"type": "grpc"}
		if serviceName != "" {
			transport["service_name"] = serviceName
		}
		out["transport"] = transport
		return nil
	case "h2", "http":
		transport := map[string]any{"type": "http"}
		if host != "" {
			transport["host"] = []string{host}
		}
		if path != "" {
			transport["path"] = path
		}
		out["transport"] = transport
		return nil
	default:
		return fmt.Errorf("unsupported node proxy transport")
	}
}

func decodeBase64Loose(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("empty base64")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if pad := len(value) % 4; pad != 0 {
		value += strings.Repeat("=", 4-pad)
	}
	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func parseURLPort(u *url.URL) (int, error) {
	if u.Port() == "" {
		return 0, fmt.Errorf("missing port")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}

func parseAnyPort(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) != v {
			return 0, fmt.Errorf("invalid port")
		}
		port := int(v)
		if port <= 0 || port > 65535 {
			return 0, fmt.Errorf("invalid port")
		}
		return port, nil
	case string:
		port, err := strconv.Atoi(v)
		if err != nil || port <= 0 || port > 65535 {
			return 0, fmt.Errorf("invalid port")
		}
		return port, nil
	default:
		return 0, fmt.Errorf("invalid port")
	}
}

func parseOptionalInt(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	case string:
		if v == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(v)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
