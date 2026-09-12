package provider

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestBuildSingBoxOutboundFromVMessLink(t *testing.T) {
	payload := map[string]any{
		"add":  "proxy.example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"aid":  "0",
		"net":  "ws",
		"host": "cdn.example.com",
		"path": "/ray",
		"tls":  "tls",
		"sni":  "sni.example.com",
	}
	data, _ := json.Marshal(payload)
	out, err := buildSingBoxOutbound("vmess://" + base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatalf("buildSingBoxOutbound() error = %v", err)
	}
	if out["type"] != "vmess" || out["server"] != "proxy.example.com" || out["server_port"] != 443 {
		t.Fatalf("unexpected vmess outbound: %#v", out)
	}
	if out["uuid"] != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("uuid not preserved: %#v", out)
	}
	transport := out["transport"].(map[string]any)
	if transport["type"] != "ws" || transport["path"] != "/ray" {
		t.Fatalf("ws transport not converted: %#v", transport)
	}
	tls := out["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "sni.example.com" {
		t.Fatalf("tls not converted: %#v", tls)
	}
}

func TestBuildSingBoxOutboundFromUppercaseVMessLink(t *testing.T) {
	payload := map[string]any{
		"add":  "proxy.example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
	}
	data, _ := json.Marshal(payload)
	out, err := buildSingBoxOutbound("VMESS://" + base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatalf("buildSingBoxOutbound() error = %v", err)
	}
	if out["type"] != "vmess" {
		t.Fatalf("uppercase vmess was not converted: %#v", out)
	}
}

func TestBuildSingBoxOutboundRejectsFractionalVMessPort(t *testing.T) {
	payload := map[string]any{
		"add":  "proxy.example.com",
		"port": 443.5,
		"id":   "11111111-1111-1111-1111-111111111111",
	}
	data, _ := json.Marshal(payload)
	if _, err := buildSingBoxOutbound("vmess://" + base64.StdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("buildSingBoxOutbound() expected fractional port error")
	}
}

func TestBuildSingBoxOutboundFromVLESSLink(t *testing.T) {
	out, err := buildSingBoxOutbound("vless://11111111-1111-1111-1111-111111111111@proxy.example.com:443?security=tls&type=grpc&serviceName=maxx&sni=sni.example.com#demo")
	if err != nil {
		t.Fatalf("buildSingBoxOutbound() error = %v", err)
	}
	if out["type"] != "vless" || out["server"] != "proxy.example.com" || out["server_port"] != 443 {
		t.Fatalf("unexpected vless outbound: %#v", out)
	}
	transport := out["transport"].(map[string]any)
	if transport["type"] != "grpc" || transport["service_name"] != "maxx" {
		t.Fatalf("grpc transport not converted: %#v", transport)
	}
}

func TestBuildSingBoxOutboundRejectsVLESSWithoutUser(t *testing.T) {
	if _, err := buildSingBoxOutbound("vless://proxy.example.com:443?security=tls"); err == nil {
		t.Fatal("buildSingBoxOutbound() expected missing vless user error")
	}
}

func TestBuildSingBoxOutboundFromTrojanLink(t *testing.T) {
	out, err := buildSingBoxOutbound("trojan://secret@proxy.example.com:443?type=ws&host=cdn.example.com&path=%2Fws&sni=sni.example.com")
	if err != nil {
		t.Fatalf("buildSingBoxOutbound() error = %v", err)
	}
	if out["type"] != "trojan" || out["password"] != "secret" {
		t.Fatalf("unexpected trojan outbound: %#v", out)
	}
}

func TestBuildSingBoxOutboundRejectsTrojanWithoutUser(t *testing.T) {
	if _, err := buildSingBoxOutbound("trojan://proxy.example.com:443?security=tls"); err == nil {
		t.Fatal("buildSingBoxOutbound() expected missing trojan user error")
	}
}

func TestBuildSingBoxOutboundFromShadowsocksLink(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:secret"))
	out, err := buildSingBoxOutbound("ss://" + encoded + "@proxy.example.com:8388#demo")
	if err != nil {
		t.Fatalf("buildSingBoxOutbound() error = %v", err)
	}
	if out["type"] != "shadowsocks" || out["method"] != "aes-128-gcm" || out["password"] != "secret" {
		t.Fatalf("unexpected shadowsocks outbound: %#v", out)
	}
}

func TestResolveProviderProxyURLPreservesOrdinaryProxyURL(t *testing.T) {
	got, err := resolveProviderProxyURL("socks5h://127.0.0.1:10808")
	if err != nil {
		t.Fatalf("resolveProviderProxyURL() error = %v", err)
	}
	if got != "socks5h://127.0.0.1:10808" {
		t.Fatalf("resolveProviderProxyURL() = %q", got)
	}
}

func TestBuildSingBoxOutboundRejectsUnsupportedTransports(t *testing.T) {
	payload := map[string]any{
		"add":  "proxy.example.com",
		"port": "443",
		"id":   "11111111-1111-1111-1111-111111111111",
		"net":  "quic",
	}
	data, _ := json.Marshal(payload)
	if _, err := buildSingBoxOutbound("vmess://" + base64.StdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("buildSingBoxOutbound() expected unsupported vmess transport error")
	}
	if _, err := buildSingBoxOutbound("vless://11111111-1111-1111-1111-111111111111@proxy.example.com:443?type=quic"); err == nil {
		t.Fatal("buildSingBoxOutbound() expected unsupported vless transport error")
	}
}

func TestBuildSingBoxOutboundRejectsUnsafeVLESSSecurity(t *testing.T) {
	for _, security := range []string{"xtls", "unknown"} {
		raw := "vless://11111111-1111-1111-1111-111111111111@proxy.example.com:443?security=" + security
		if _, err := buildSingBoxOutbound(raw); err == nil {
			t.Fatalf("buildSingBoxOutbound(%s) expected unsupported security error", security)
		}
	}
}

func TestBuildSingBoxOutboundRejectsMissingURLPort(t *testing.T) {
	cases := []string{
		"vless://11111111-1111-1111-1111-111111111111@proxy.example.com?security=tls",
		"trojan://secret@proxy.example.com?type=ws",
		"ss://YWVzLTEyOC1nY206c2VjcmV0@proxy.example.com",
	}
	for _, raw := range cases {
		if _, err := buildSingBoxOutbound(raw); err == nil {
			t.Fatalf("buildSingBoxOutbound(%s) expected missing port error", raw)
		}
	}
}
