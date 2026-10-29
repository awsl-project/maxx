package e2e_test

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func saveFullPrivacy(t *testing.T, env *ProxyTestEnv, cfg privacy.Config) {
	t.Helper()
	resp := env.AdminPut("/api/admin/privacy", map[string]any{"config": cfg})
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("save privacy: %d %s", resp.StatusCode, body)
	}
}
func TestFullPrivacyHTTPAndSSERoundTrip(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("direct=%t/stream=%t", direct, stream), func(t *testing.T) {
				env := NewProxyTestEnv(t)
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					raw, _ := io.ReadAll(r.Body)
					if strings.Contains(string(raw), "Nightjar") || strings.Contains(string(raw), "person@corp-real.com") {
						t.Error("original reached upstream")
					}
					var request struct {
						Messages []struct {
							Content string `json:"content"`
						} `json:"messages"`
					}
					if err := json.Unmarshal(raw, &request); err != nil || len(request.Messages) == 0 {
						t.Error("bad upstream body")
						w.WriteHeader(400)
						return
					}
					text := request.Messages[0].Content
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						at := len(text) / 2
						for _, chunk := range []string{text[:at], text[at:]} {
							fmt.Fprintf(w, "data: {\"id\":\"chat-full\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", chunk)
							w.(http.Flusher).Flush()
						}
						io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"id":"chat-full","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, text)
				}))
				defer upstream.Close()
				provider := createProvider(t, env, "full-privacy", upstream.URL, []string{"openai"})
				createRoute(t, env, "openai", provider)
				project := createPrivacyTestProject(t, env, "full-privacy")
				cfg := privacy.DefaultConfig()
				cfg.Enabled = true
				cfg.Personal = true
				cfg.Words = []string{"Nightjar"}
				saveFullPrivacy(t, env, cfg)
				body := openaiRequest("gpt-4o")
				body["stream"] = stream
				body["messages"] = []any{map[string]any{"role": "user", "content": "Nightjar person@corp-real.com"}}
				path := "/project/full-privacy/v1/chat/completions"
				if direct {
					path = fmt.Sprintf("/provider/%d/v1/chat/completions", provider)
				}
				resp := env.ProxyPost(path, body, map[string]string{"Authorization": "Bearer arbitrary-client-key", "X-Maxx-Project-ID": fmt.Sprint(project.ID)})
				result, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 || !strings.Contains(string(result), "Nightjar") || !strings.Contains(string(result), "person@corp-real.com") || calls.Load() != 1 {
					t.Fatalf("response %d: %s calls=%d", resp.StatusCode, result, calls.Load())
				}
				if stream && !strings.Contains(string(result), "[DONE]") {
					t.Fatal("stream terminal lost")
				}
				stored := waitPrivacyRequest(t, env)
				if stored.Status != "COMPLETED" {
					t.Fatalf("status %s %s", stored.Status, stored.Error)
				}
				var rows []sqlite.ProxyRequest
				if err := env.DB.GormDB().Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					if strings.Contains(string(row.RequestInfo), "Nightjar") || strings.Contains(string(row.ResponseInfo), "person@corp-real.com") {
						t.Fatal("original persisted in request archive")
					}
				}
				recordsResp := env.AdminGet("/api/admin/privacy/records")
				defer recordsResp.Body.Close()
				var records privacy.RecordsPage
				if json.NewDecoder(recordsResp.Body).Decode(&records) != nil || records.Total != 1 || len(records.Items[0].Values) < 2 {
					t.Fatalf("missing records: %+v", records)
				}
				for _, v := range records.Items[0].Values {
					if v.Restored != 1 {
						t.Fatalf("restore count=%d: %+v", v.Restored, v)
					}
				}
			})
		}
	}
}
func TestFullPrivacyModesBypassAndUnavailableModel(t *testing.T) {
	env := NewProxyTestEnv(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	provider := createProvider(t, env, "mode-provider", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	createPrivacyTestProject(t, env, "full-modes")
	cfg := privacy.DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{"Nightjar"}
	cfg.Mode = "block"
	saveFullPrivacy(t, env, cfg)
	body := openaiRequest("gpt-4o")
	body["messages"] = []any{map[string]any{"role": "user", "content": "Nightjar"}}
	send := func(want int) {
		t.Helper()
		resp := env.ProxyPost("/project/full-modes/v1/chat/completions", body, nil)
		result, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("status=%d want=%d: %s", resp.StatusCode, want, result)
		}
	}
	send(400)
	if calls.Load() != 0 {
		t.Fatal("blocked request reached upstream")
	}
	cfg.Mode = "warn"
	saveFullPrivacy(t, env, cfg)
	send(200)
	cfg.Mode = "block"
	cfg.BypassProviders = []uint64{provider}
	saveFullPrivacy(t, env, cfg)
	send(200)
	cfg.BypassProviders = nil
	cfg.Model.Enabled = true
	t.Setenv("MAXX_PRIVACY_CACHE_DIR", t.TempDir())
	saveFullPrivacy(t, env, cfg)
	send(503)
	if calls.Load() != 2 {
		t.Fatal("failed model silently forwarded")
	}
	cfg.Enabled = false
	saveFullPrivacy(t, env, cfg)
	send(200)
	if calls.Load() != 3 {
		t.Fatal("disabled model affected old transport")
	}
}
func TestFullPrivacyGlobalConfigurationAndPreview(t *testing.T) {
	env := NewProxyTestEnv(t)
	project := createPrivacyTestProject(t, env, "config-preview")
	other := createPrivacyTestProject(t, env, "config-other")
	cfg := privacy.DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{"Nightjar"}
	cfg.WordsAnyCase = true
	cfg.Personal = true
	saveFullPrivacy(t, env, cfg)
	endpoint := "/api/admin/privacy"
	resp := env.AdminGet(endpoint)
	var got privacy.Config
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if !got.Enabled || !got.Personal || len(got.Words) != 1 {
		t.Fatalf("config roundtrip: %+v", got)
	}
	preview := env.AdminPost(endpoint+"/preview", map[string]any{"config": cfg, "text": "NIGHTJAR person@corp-real.com"})
	var result privacy.Preview
	json.NewDecoder(preview.Body).Decode(&result)
	preview.Body.Close()
	if len(result.Hits) < 2 || strings.Contains(result.Body, "NIGHTJAR") {
		t.Fatalf("bad preview: %+v", result)
	}
	records := env.AdminGet(endpoint + "/records")
	var page privacy.RecordsPage
	json.NewDecoder(records.Body).Decode(&page)
	records.Body.Close()
	if page.Total != 0 {
		t.Fatal("preview wrote history")
	}
	projectResp := env.AdminGet(fmt.Sprintf("/api/admin/projects/%d", project.ID))
	raw, _ := io.ReadAll(projectResp.Body)
	projectResp.Body.Close()
	if strings.Contains(string(raw), "Nightjar") || strings.Contains(string(raw), "words") {
		t.Fatal("general project API exposed policy contents")
	}
	otherResp := env.AdminGet(fmt.Sprintf("/api/admin/projects/%d/privacy", other.ID))
	AssertStatus(t, otherResp, http.StatusNotFound)
	otherResp.Body.Close()
	legacy := env.AdminPut(fmt.Sprintf("/api/admin/projects/%d", project.ID), map[string]any{"name": "renamed", "slug": "config-preview"})
	legacy.Body.Close()
	resp = env.AdminGet(endpoint)
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if !got.Enabled || len(got.Words) != 1 {
		t.Fatal("general editor discarded full privacy policy")
	}
	unauth, _ := http.Get(env.Server.URL + endpoint + "/records")
	unauth.Body.Close()
	if unauth.StatusCode != 401 {
		t.Fatalf("unauthenticated history status=%d", unauth.StatusCode)
	}
}

func TestFullPrivacyWebSocketTurnRestoration(t *testing.T) {
	env := NewProxyTestEnv(t)
	createPrivacyTestProject(t, env, "full-ws")
	var originalSent atomic.Bool
	var connections atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			return
		}
		defer conn.Close()
		connections.Add(1)
		for {
			_, frame, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if strings.Contains(string(frame), "Nightjar") {
				originalSent.Store(true)
			}
			text := gjson.GetBytes(frame, "input.0.content").String()
			at := len(text) / 2
			for _, delta := range []string{text[:at], text[at:]} {
				event := fmt.Sprintf(`{"type":"response.output_text.delta","item_id":"item-1","output_index":0,"content_index":0,"delta":%q}`, delta)
				if conn.WriteMessage(websocket.TextMessage, []byte(event)) != nil {
					return
				}
			}
			if conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_full","model":"gpt-test","output":[]}}`)) != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	response := env.AdminPost("/api/admin/providers", map[string]any{"name": "full-ws", "type": "custom", "config": map[string]any{"custom": map[string]any{"baseURL": upstream.URL + "/v1", "apiKey": "sk-mock-test-key", "responsesWebSocket": true}}, "supportedClientTypes": []string{"codex"}})
	AssertStatus(t, response, 201)
	var provider domain.Provider
	DecodeJSON(t, response, &provider)
	createNativeRoute(t, env, "codex", provider.ID, 1)
	cfg := privacy.DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{"Nightjar"}
	saveFullPrivacy(t, env, cfg)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(env.Server.URL, "http")+"/project/full-ws/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for turn := 0; turn < 2; turn++ {
		if turn == 1 {
			cfg.Enabled = false
			saveFullPrivacy(t, env, cfg)
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[{"role":"user","content":"Nightjar"}]}`)) != nil {
			t.Fatal("write turn")
		}
		var text strings.Builder
		for {
			_, event, err := conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			typ := gjson.GetBytes(event, "type").String()
			if typ == "error" {
				t.Fatal(string(event))
			}
			if typ == "response.output_text.delta" {
				text.WriteString(gjson.GetBytes(event, "delta").String())
			}
			if typ == "response.completed" {
				break
			}
		}
		if text.String() != "Nightjar" {
			t.Fatalf("turn %d restoration=%q", turn, text.String())
		}
		if originalSent.Load() != (turn == 1) {
			t.Fatal("turn did not apply the global policy")
		}
	}
	if connections.Load() != 1 {
		t.Fatalf("privacy changed native session reuse: %d", connections.Load())
	}
}

func TestFullPrivacyResponseConversionRestoresAfterConversion(t *testing.T) {
	env := NewProxyTestEnv(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if strings.Contains(string(raw), "Nightjar") {
			t.Error("private original reached conversion provider")
		}
		text := gjson.GetBytes(raw, "messages.0.content").String()
		if gjson.GetBytes(raw, "messages.0.content").IsArray() {
			text = gjson.GetBytes(raw, "messages.0.content.0.text").String()
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"msg_privacy","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":%q}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`, text)
	}))
	defer upstream.Close()
	provider := createProvider(t, env, "conversion-private", upstream.URL, []string{"claude"})
	createRoute(t, env, "openai", provider)
	createPrivacyTestProject(t, env, "full-conversion")
	cfg := privacy.DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{"Nightjar"}
	saveFullPrivacy(t, env, cfg)
	body := openaiRequest("gpt-4o")
	body["messages"] = []any{map[string]any{"role": "user", "content": "Nightjar"}}
	resp := env.ProxyPost("/project/full-conversion/v1/chat/completions", body, nil)
	result, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(result), "Nightjar") {
		t.Fatalf("converted response %d: %s", resp.StatusCode, result)
	}
}
