package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestGrokConfigRoundTripAndSecretPreserve(t *testing.T) {
	env := NewProxyTestEnv(t)

	cresp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "grok-cfg",
		"type": "grok",
		"config": map[string]any{
			"disableErrorCooldown": true,
			"grok": map[string]any{
				"type":          "xai",
				"authKind":      "oauth",
				"email":         "user@example.com",
				"accessToken":   "access-token",
				"refreshToken":  "refresh-token",
				"idToken":       "id-token",
				"baseURL":       "https://cli-chat-proxy.grok.com/v1",
				"tokenEndpoint": "https://example.test/token",
			},
		},
	})
	if cresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(cresp.Body)
		t.Fatalf("create: status=%d body=%s", cresp.StatusCode, b)
	}
	var created struct {
		ID                   uint64   `json:"id"`
		SupportedClientTypes []string `json:"supportedClientTypes"`
	}
	DecodeJSON(t, cresp, &created)
	if len(created.SupportedClientTypes) != 1 || created.SupportedClientTypes[0] != "openai" {
		t.Fatalf("grok supportedClientTypes = %v, want [openai]", created.SupportedClientTypes)
	}

	uresp := env.AdminPut(fmt.Sprintf("/api/admin/providers/%d", created.ID), map[string]any{
		"name": "grok-cfg-updated",
		"type": "grok",
		"config": map[string]any{
			"disableErrorCooldown": false,
			"grok": map[string]any{
				"type":         "xai",
				"authKind":     "oauth",
				"email":        "next@example.com",
				"accessToken":  "",
				"refreshToken": "",
				"idToken":      "",
			},
		},
		"supportedClientTypes": []string{"claude", "openai"},
	})
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		t.Fatalf("update: status=%d body=%s", uresp.StatusCode, b)
	}
	uresp.Body.Close()

	gresp := env.AdminGet(fmt.Sprintf("/api/admin/providers/%d", created.ID))
	var got struct {
		Type                 string   `json:"type"`
		SupportedClientTypes []string `json:"supportedClientTypes"`
		Config               struct {
			DisableErrorCooldown bool `json:"disableErrorCooldown"`
			Grok                 *struct {
				Email        string `json:"email"`
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
				IDToken      string `json:"idToken"`
			} `json:"grok"`
		} `json:"config"`
	}
	DecodeJSON(t, gresp, &got)
	if got.Type != "grok" || got.Config.Grok == nil {
		t.Fatalf("unexpected provider after update: %+v", got)
	}
	if got.Config.Grok.Email != "next@example.com" {
		t.Errorf("email = %q, want next@example.com", got.Config.Grok.Email)
	}
	if got.Config.Grok.AccessToken != "access-token" || got.Config.Grok.RefreshToken != "refresh-token" || got.Config.Grok.IDToken != "id-token" {
		t.Errorf("blank update should preserve stored tokens, got access=%q refresh=%q id=%q", got.Config.Grok.AccessToken, got.Config.Grok.RefreshToken, got.Config.Grok.IDToken)
	}
	if got.Config.DisableErrorCooldown {
		t.Error("disableErrorCooldown should round-trip false after update")
	}
	if len(got.SupportedClientTypes) != 1 || got.SupportedClientTypes[0] != "openai" {
		t.Errorf("grok update should force supportedClientTypes [openai], got %v", got.SupportedClientTypes)
	}
}

func TestGrokExportSkipsBlackBoxAndIncludesExportableConfig(t *testing.T) {
	env := NewProxyTestEnv(t)

	for _, tc := range []struct {
		name              string
		excludeFromExport bool
	}{
		{name: "grok-export", excludeFromExport: false},
		{name: "grok-hidden", excludeFromExport: true},
	} {
		resp := env.AdminPost("/api/admin/providers", map[string]any{
			"name": tc.name,
			"type": "grok",
			"config": map[string]any{"grok": map[string]any{
				"type":         "xai",
				"authKind":     "oauth",
				"accessToken":  "access-" + tc.name,
				"refreshToken": "refresh-" + tc.name,
			}},
			"excludeFromExport": tc.excludeFromExport,
		})
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("create %s: status=%d body=%s", tc.name, resp.StatusCode, b)
		}
		resp.Body.Close()
	}

	eresp := env.AdminGet("/api/admin/providers/export")
	var exported []map[string]any
	DecodeJSON(t, eresp, &exported)
	var found map[string]any
	for _, provider := range exported {
		switch provider["name"] {
		case "grok-hidden":
			t.Fatal("excludeFromExport Grok provider leaked into export")
		case "grok-export":
			found = provider
		}
	}
	if found == nil {
		t.Fatal("exportable Grok provider missing from export")
	}
	cfg, _ := found["config"].(map[string]any)
	grok, _ := cfg["grok"].(map[string]any)
	if grok == nil || grok["accessToken"] != "access-grok-export" || grok["refreshToken"] != "refresh-grok-export" {
		t.Fatalf("export should include Grok tokens for re-import, got config=%v", cfg)
	}
}
