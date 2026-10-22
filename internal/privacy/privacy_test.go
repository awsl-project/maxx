package privacy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"
)

const testKey = "sk-proj-testtesttesttesttesttesttesttest"

func TestCredentialRules(t *testing.T) {
	// Generate a synthetic JWT with a dummy signature; never embed a real token.
	jwt := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test"}`)) + "." + strings.Repeat("test", 4)

	for _, secret := range []string{testKey, "ghp_abcdefghijklmnopqrstuvwxyz123456", "github_pat_abcdefghijklmnopqrstuvwxyz123456", "AKIAIOSFODNN7EXAMPLE", jwt, "-----BEGIN PRIVATE KEY-----\nprivate material\n-----END PRIVATE KEY-----", "API_KEY=example_example_example", "postgres://user:secret123@db.local/db"} {
		t.Run(secret[:8], func(t *testing.T) {
			if Detect(secret) == "" {
				t.Fatal("credential missed")
			}
			if got := ScrubText(secret); got == secret || !strings.Contains(got, Redacted) {
				t.Fatalf("not scrubbed: %q", got)
			}
		})
	}
	ordinary := "max_tokens=4096 token_count=100 use model deepseek/deepseek-v4.1-flash, request id sk-short"
	if Detect(ordinary) != "" || ScrubText(ordinary) != ordinary {
		t.Fatal("ordinary protocol fields changed")
	}
	if got := ScrubText("postgres://alice:hunter123@localhost/db"); got != "postgres://alice:[REDACTED]@localhost/db" {
		t.Fatal(got)
	}
}

func TestScrubBody(t *testing.T) {
	body := `{"id":9007199254740993,"max_tokens":20,"password":"short","messages":[{"content":"` + testKey + `"},{"content":"{\"access_token\":\"tiny\"}"}]}`
	got := ScrubBody(body)
	if !json.Valid([]byte(got)) || strings.Contains(got, testKey) || strings.Contains(got, "short") || strings.Contains(got, "tiny") {
		t.Fatalf("unsafe JSON: %s", got)
	}
	if !strings.Contains(got, "9007199254740993") || !strings.Contains(got, `"max_tokens":20`) {
		t.Fatal("numbers changed")
	}
	if got := ScrubBody(`{"` + testKey + `":"value"}`); strings.Contains(got, testKey) {
		t.Fatal("credential in JSON key leaked")
	}
	if ScrubBody("plain safe text") != "plain safe text" {
		t.Fatal("safe text changed")
	}
}

func TestScrubSplitStreams(t *testing.T) {
	for _, format := range []string{"sse", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			var frames []string
			// Even a key sent one character per event must not survive in an archive.
			for _, ch := range testKey {
				body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": string(ch)}}}})
				if format == "sse" {
					body = append([]byte("data: "), body...)
				}
				frames = append(frames, string(body))
			}
			archive := strings.Join(frames, "\n\n") + "\n\ndata: [DONE]\n\n"
			got := ScrubBody(archive)
			if got != "[REDACTED: credential in streamed content]" {
				t.Fatalf("split credential persisted: %s", got)
			}
		})
	}
	safe := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"
	if got := ScrubBody(safe); got != safe {
		t.Fatalf("safe SSE changed: %q", got)
	}
}

func TestInspectRequest(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"user","content":"` + testKey + `"}]}`,
		`{"messages":[{"role":"tool","content":"{\"password\":\"tiny\"}"}]}`,
		`{"messages":[{"role":"assistant","tool_calls":[{"function":{"name":"run","arguments":"{\"api_key\":\"tiny\"}"}}]}]}`,
		`{"contents":[{"parts":[{"text":"` + testKey + `"}]}]}`,
		`{"input":[{"type":"function_call_output","call_id":"ok","output":"API_KEY=example_example_example"}]}`,
		`{"system":"password=abcdefghijk","messages":[]}`,
		`{"messages":[{"role":"tool","content":"{\"data\":{\"password\":\"tiny\"}}"}]}`,
	} {
		kind, err := InspectRequest([]byte(body), "application/json")
		if err != nil || kind == "" {
			t.Fatalf("missed payload %s: kind=%s err=%v", body, kind, err)
		}
	}
	safe := `{"model":"` + testKey + `","max_tokens":12,"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + testKey + `"}},{"type":"text","text":"explain this"}]},{"role":"assistant","tool_calls":[{"id":"` + testKey + `","function":{"name":"` + testKey + `","arguments":"{\"max_tokens\":4}"}}]}],"tools":[{"name":"search","parameters":{"properties":{"password":{"type":"string"}}}}]}`
	if kind, err := InspectRequest([]byte(safe), "application/json"); kind != "" || err != nil {
		t.Fatalf("protocol or schema mistaken for content: %s %v", kind, err)
	}
	for _, body := range []string{`{`, `[]`, `{"messages":[]} {}`} {
		if _, err := InspectRequest([]byte(body), "application/json"); err == nil {
			t.Fatalf("invalid input accepted: %s", body)
		}
	}
	// Detection never leaves state that can affect a subsequent request/tenant.
	InspectRequest([]byte(`{"input":"`+testKey+`"}`), "application/json")
	if kind, err := InspectRequest([]byte(`{"input":"hello"}`), "application/json"); kind != "" || err != nil {
		t.Fatal("cross-request state")
	}
}

func TestInspectMultipartTextOnly(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	file, _ := w.CreateFormFile("image", "sample.png")
	file.Write([]byte(testKey))
	w.WriteField("prompt", "describe this image")
	w.Close()
	if kind, err := InspectRequest(body.Bytes(), w.FormDataContentType()); kind != "" || err != nil {
		t.Fatalf("binary file inspected: %s %v", kind, err)
	}
	body.Reset()
	w = multipart.NewWriter(&body)
	w.WriteField("prompt", testKey)
	w.Close()
	if kind, err := InspectRequest(body.Bytes(), w.FormDataContentType()); kind == "" || err != nil {
		t.Fatalf("text prompt missed: %s %v", kind, err)
	}
	if _, err := InspectRequest(body.Bytes(), "multipart/form-data"); err == nil {
		t.Fatal("missing boundary accepted")
	}
}
