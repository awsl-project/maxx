package privacy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/url"
	"strings"
	"testing"
)

const testKey = "sk-proj-testtesttesttesttesttesttesttest"

func TestCredentialRules(t *testing.T) {
	// Generate a synthetic JWT with a dummy signature; never embed a real token.
	jwt := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"test"}`)) + "." + strings.Repeat("test", 4)

	for _, secret := range []string{testKey, "ghp_abcdefghijklmnopqrstuvwxyz123456", "github_pat_" + strings.Repeat("test", 10), "AKIAIOSFODNN7EXAMPLE", jwt, "-----BEGIN PRIVATE KEY-----\nprivate material\n-----END PRIVATE KEY-----", "API_KEY=example_example_example", "postgres://user:secret123@db.local/db"} {
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

func TestOrdinaryTextAndCodeAreNotCredentials(t *testing.T) {
	for _, text := range []string{
		"Explain basic internationalization", "basic responsibilities",
		`password = os.getenv("X")`, `api_key = config.api_key`, `api_key = ${CONFIG_API_KEY}`,
	} {
		t.Run(text, func(t *testing.T) {
			if kind := Detect(text); kind != "" {
				t.Errorf("ordinary text detected as %s", kind)
			}
			if got := ScrubText(text); got != text {
				t.Errorf("ordinary text scrubbed: %s", got)
			}

			first, _ := json.Marshal(map[string]any{"delta": text[:len(text)/2]})
			second, _ := json.Marshal(map[string]any{"delta": text[len(text)/2:]})
			stream := "data: " + string(first) + "\n\ndata: " + string(second) + "\n\n"
			if got := ScrubBody(stream); got != stream {
				t.Error("ordinary fragmented stream scrubbed")
			}
			body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": text}}})
			if kind, err := InspectRequest(body, "application/json"); kind != "" || err != nil {
				t.Errorf("ordinary request blocked: %s %v", kind, err)
			}
			var stored struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal([]byte(ScrubBody(string(body))), &stored); err != nil || stored.Messages[0].Content != text {
				t.Error("ordinary archive changed")
			}
		})
	}
}

func TestToolObjectsDoNotSkipArbitraryKeys(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"assistant","content":[{"type":"tool_use","name":"run","input":{"data":"` + testKey + `"}}]}]}`,
		`{"contents":[{"parts":[{"functionCall":{"name":"run","args":{"id":"` + testKey + `"}}}]}]}`,
		`{"contents":[{"parts":[{"functionResponse":{"name":"run","response":{"type":"` + testKey + `"}}}]}]}`,
	} {
		if kind, err := InspectRequest([]byte(body), "application/json"); kind == "" || err != nil {
			t.Errorf("tool credential missed: %s %v", kind, err)
		}
	}
}

func TestAuthorizationAndLiteralPasswordsRemainProtected(t *testing.T) {
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("dummy:example-password"))
	basicAlpha := "Basic " + base64.RawStdEncoding.EncodeToString([]byte("aaaaaaa:aaaaaaa"))
	for _, text := range []string{basic, basicAlpha, "Bearer " + strings.Repeat("test", 8), `password = "internationalization"`, `password = "config.api_key"`, `password = "os.getenv("`} {
		if Detect(text) == "" {
			t.Errorf("literal credential missed: %s", text)
		}
		if ScrubText(text) == text {
			t.Errorf("literal credential not scrubbed: %s", text)
		}
	}
}

func TestCodeReferenceDoesNotHideALaterCredential(t *testing.T) {
	text := `password=os.getenv("X"); API_KEY=example_example_example`
	if Detect(text) == "" {
		t.Fatal("later credential missed")
	}
	if got := ScrubText(text); !strings.Contains(got, `password=os.getenv("X")`) || strings.Contains(got, "example_example_example") {
		t.Fatalf("wrong replacement: %s", got)
	}
}

func TestPasswordPunctuation(t *testing.T) {
	for _, text := range []string{
		`password="abc;Example456"`, `password=Abcd$efg123`, `password="tiny"`, `password="abc;Example456`,
		`password="ab'cd;Example456"`, `password="abc\";Example456"`,
		`password='abc;Example456'`,
	} {
		if Detect(text) == "" {
			t.Errorf("credential missed: %s", text)
		}
		if got := ScrubText(text); strings.Contains(got, "tiny") {
			t.Errorf("short quoted password left behind: %s", got)
		}
		if got := ScrubText(text); strings.Contains(got, "Example456") || strings.Contains(got, "Abcd$efg123") {
			t.Errorf("credential left behind: %s", got)
		}
		body, _ := json.Marshal(map[string]any{"input": text})
		if kind, err := InspectRequest(body, "application/json"); kind == "" || err != nil {
			t.Errorf("credential forwarded: %s %v", kind, err)
		}
		if got := ScrubBody(string(body)); strings.Contains(got, "Example456") || strings.Contains(got, "Abcd$efg123") {
			t.Errorf("archive contains credential: %s", got)
		}
	}
}

func TestAdditionalCredentialFormats(t *testing.T) {
	for _, secret := range []string{
		"xoxb-" + strings.Repeat("test", 8),
		"sk_live_" + strings.Repeat("test", 8),
		"hf_" + strings.Repeat("test", 8),
		"gsk_" + strings.Repeat("test", 10),
		"xai-" + strings.Repeat("test", 10),
		"npm_" + strings.Repeat("test", 9),
		"pypi-" + strings.Repeat("test", 13),
		"dop_v1_" + strings.Repeat("abcd", 16),
		"SG." + strings.Repeat("a", 22) + "." + strings.Repeat("b", 43),
		"AWS_SECRET_ACCESS_KEY=example_example_example",
		"AZURE_OPENAI_API_KEY=example_example_example",
	} {
		if Detect(secret) == "" || ScrubText(secret) == secret {
			t.Errorf("credential format missed: %s", secret)
		}
	}
	for _, field := range []string{"aws_secret_access_key", "awsSessionToken", "db_password", "token", "AZURE_OPENAI_API_KEY"} {
		body, _ := json.Marshal(map[string]any{"input": map[string]any{field: "tiny"}})
		if kind, err := InspectRequest(body, "application/json"); kind == "" || err != nil {
			t.Errorf("credential field missed: %s", field)
		}
		if strings.Contains(ScrubBody(string(body)), "tiny") {
			t.Errorf("credential field not scrubbed: %s", field)
		}
	}
}

func TestArchiveURLAndHeaderCredentials(t *testing.T) {
	for _, raw := range []string{
		"https://alice:tiny@example.com/v1?api_key=tiny&safe=hello",
		"/v1/chat/completions?key=tiny&key=other&max_tokens=123&safe=hello",
		"/v1?access_token=%74%69%6e%79&safe=hello",
		"/v1?debug=" + testKey + "&safe=hello",
	} {
		cleaned := ScrubURL(raw)
		if _, err := url.Parse(cleaned); err != nil {
			t.Errorf("invalid archived URL: %s", cleaned)
		}
		if got := ScrubURL(cleaned); got != cleaned {
			t.Errorf("URL scrubbing not idempotent: %s", got)
		}
		if strings.Contains(cleaned, "tiny") || strings.Contains(cleaned, "other") || strings.Contains(cleaned, testKey) || !strings.Contains(cleaned, "safe=hello") {
			t.Errorf("unsafe/unusable URL: %s", cleaned)
		}
	}
	safe := "/v1?b=a%20b&a=1&max_tokens=123"
	if ScrubURL(safe) != safe || ScrubURL("") != "" {
		t.Fatal("credential-free URL changed")
	}
	for _, raw := range []string{"http://[broken", "/v1?api_key=tiny;unsafe"} {
		if ScrubURL(raw) != Redacted {
			t.Fatal("malformed credential URL retained")
		}
	}
	for _, name := range []string{"X-Service-Secret", "X-Session-Token", "X-Database-Password", "X-Service-Api-Key", "Authorization", "Cookie"} {
		if got := ScrubHeader(name, "tiny"); got != Redacted {
			t.Errorf("header credential not scrubbed: %s", name)
		}
	}
	if got := ScrubHeader("X-Debug", testKey); got != Redacted {
		t.Fatal("credential in ordinary header missed")
	}
	if got := ScrubHeader("Content-Type", "application/json"); got != "application/json" {
		t.Fatal("ordinary header changed")
	}
}

func TestArchivePreservesOpaqueMediaAndScrubsSurroundingText(t *testing.T) {
	media := "data:image/png;base64," + "AKIAIOSFODNN7EXAMPLE"
	body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"content": []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": media}},
		map[string]any{"type": "text", "text": testKey},
	}}}})
	got := ScrubBody(string(body))
	if !strings.Contains(got, media) || strings.Contains(got, testKey) {
		t.Fatalf("media corrupted or text not scrubbed: %s", got)
	}
	text, _ := json.Marshal(map[string]any{"content": "data:image/png;base64,not-base64 " + testKey})
	if strings.Contains(ScrubBody(string(text)), testKey) {
		t.Fatal("data URL prefix hid ordinary secret text")
	}
}

func TestMultipartStructuredTextIsInspected(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("prompt", `{"password":"tiny"}`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if kind, err := InspectRequest(body.Bytes(), writer.FormDataContentType()); kind == "" || err != nil {
		t.Fatalf("structured prompt forwarded: %s %v", kind, err)
	}
}

func TestTruncatedJSONDoesNotExposeShortPasswords(t *testing.T) {
	body := `{"password":"tiny","more":"` + strings.Repeat("x", 100) + `...<body truncated>`
	if got := ScrubBody(body); strings.Contains(got, "tiny") {
		t.Fatal("truncated archive leaked a short credential")
	}
}

func TestPGPPrivateKeyBlock(t *testing.T) {
	label := "PGP PRIVATE KEY BLOCK"
	text := "-----BEGIN " + label + "-----\nexample\n-----END " + label + "-----"
	if Detect(text) == "" || ScrubText(text) != Redacted {
		t.Fatal("PGP private key not protected")
	}
}

func TestAlreadyRedactedToolResultsRemainUsable(t *testing.T) {
	for _, text := range []string{`API_KEY=[REDACTED]`, `password="[REDACTED]"`, `{"access_token":"[REDACTED]"}`} {
		if kind := Detect(text); kind != "" {
			t.Errorf("redacted value detected as %s", kind)
		}
		body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "tool", "content": text}}})
		if kind, err := InspectRequest(body, "application/json"); kind != "" || err != nil {
			t.Errorf("redacted tool output blocked: %s %v", kind, err)
		}
		if cleaned := ScrubText(text); cleaned != text {
			t.Errorf("redacted text changed: %s", cleaned)
		}
	}
	body := []byte(`{"input":{"access_token":"[REDACTED]"}}`)
	if kind, err := InspectRequest(body, "application/json"); kind != "" || err != nil {
		t.Errorf("structured redacted output blocked: %s %v", kind, err)
	}
	if kind := Detect(`API_KEY=[REDACTED]; password=example_example_example`); kind == "" {
		t.Fatal("redacted marker hid a later credential")
	}
}

func TestCamelCaseCredentialsAndBooleanFlags(t *testing.T) {
	for _, key := range []string{"adminPassword", "smtpPassword", "clientToken", "stripeApiKey"} {
		body, _ := json.Marshal(map[string]any{"input": map[string]any{key: "tiny"}})
		if kind, err := InspectRequest(body, "application/json"); kind == "" || err != nil {
			t.Errorf("camel-case field missed: %s", key)
		}
		if strings.Contains(ScrubBody(string(body)), "tiny") {
			t.Errorf("camel-case archive leaked: %s", key)
		}
		if text := key + "=example_example_example"; Detect(text) == "" || ScrubText(text) == text {
			t.Errorf("camel-case assignment missed: %s", key)
		}
	}
	body := []byte(`{"input":{"hasPassword":true,"hasApiKey":false,"token_count":8,"max_tokens":20}}`)
	if kind, err := InspectRequest(body, "application/json"); kind != "" || err != nil {
		t.Errorf("metadata flags rejected: %s %v", kind, err)
	}
	if got := ScrubBody(string(body)); !strings.Contains(got, `"hasPassword":true`) || !strings.Contains(got, `"max_tokens":20`) {
		t.Fatal("metadata flags changed")
	}
}

func TestFixedLengthKeysEndingInHyphen(t *testing.T) {
	for _, key := range []string{"AIza" + strings.Repeat("a", 34) + "-", "SG." + strings.Repeat("a", 22) + "." + strings.Repeat("b", 42) + "-"} {
		for _, suffix := range []string{"", " ", `"`} {
			text := key + suffix
			if Detect(text) == "" {
				t.Error("fixed-length key missed")
			}
			if got := ScrubText(text); got != Redacted+suffix {
				t.Errorf("key not fully scrubbed: %s", got)
			}
		}
		if Detect(key+"longer") != "" {
			t.Error("invalid longer token mistaken for fixed-length key")
		}
	}
}

func TestLiteralParenthesesAndFunctionCalls(t *testing.T) {
	text := "password=Abc(123)xyz!"
	if Detect(text) == "" || strings.Contains(ScrubText(text), "Abc(123)xyz!") {
		t.Fatal("literal password missed")
	}
	body, _ := json.Marshal(map[string]any{"input": text})
	if kind, err := InspectRequest(body, "application/json"); kind == "" || err != nil {
		t.Fatal("literal password forwarded")
	}
	if strings.Contains(ScrubBody(string(body)), "Abc(123)xyz!") {
		t.Fatal("literal password archived")
	}
	for _, code := range []string{`password=os.getenv("X")`, `password=get_password(config)`, `password=wrap(get_password())`, `api_key=config.get_key().value`, `password=$(get_password)`} {
		if Detect(code) != "" || ScrubText(code) != code {
			t.Errorf("code reference rejected: %s", code)
		}
	}
}
