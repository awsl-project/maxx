package privacy

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

type memoryPrivacySettings struct{ raw string }

func (m *memoryPrivacySettings) Get(string) (string, error)     { return m.raw, nil }
func (m *memoryPrivacySettings) Set(_ string, raw string) error { m.raw = raw; return nil }
func configuredService(t *testing.T, cfg Config) *Service {
	t.Helper()
	svc := NewService(&memoryPrivacySettings{})
	if _, err := svc.SetConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFullPolicyMaskWarnBlockAndAllow(t *testing.T) {
	for _, mode := range []string{"mask", "warn", "block"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Enabled, cfg.Mode = true, mode
			svc := configuredService(t, cfg)
			body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"` + testKey + `"}]}`)
			x, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
			if x == nil || (rejection != nil) != (mode == "block") || len(x.Hits) == 0 {
				t.Fatalf("mode %s: exchange=%+v rejection=%+v", mode, x, rejection)
			}
			if mode == "warn" {
				if !bytes.Equal(x.Body, body) || x.HasMappings() {
					t.Fatal("warn mode changed transport bytes or retained a mapping")
				}
			} else if strings.Contains(string(x.Body), testKey) || string(x.RestoreState().RestoreJSON(x.Body)) != string(body) {
				t.Fatal("mask/restore roundtrip failed")
			}
			cfg.Allow = []string{testKey}
			svc.SetConfiguration(cfg)
			x, rejection = svc.Prepare(context.Background(), 1, body, body, "application/json")
			if rejection != nil || !bytes.Equal(x.Body, body) || len(x.Hits) != 0 {
				t.Fatal("allowlist did not preserve the whole value")
			}
		})
	}
}

func TestFullPolicyWordsCustomRulesAndFieldContext(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled, cfg.WordsAnyCase, cfg.WordsWhole = true, true, true
	cfg.Words = []string{"Nightjar"}
	cfg.Rules = []redact.Rule{{Kind: "CORPORATE", Prefix: "corp_"}}
	svc := configuredService(t, cfg)
	body := []byte(`{"model":"Nightjar","messages":[{"role":"user","content":"NIGHTJAR XNightjarY corp_Abc123456789"},{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"tool","arguments":"{\"password\":\"tiny\"}"}}]}]}`)
	x, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil {
		t.Fatalf("unexpected rejection: %+v", rejection)
	}
	for _, value := range []string{"NIGHTJAR", "corp_Abc123456789", "tiny"} {
		if strings.Contains(string(x.Body), value) {
			t.Errorf("private value was not masked: %s", value)
		}
	}
	if !strings.Contains(string(x.Body), `"model":"Nightjar"`) || !strings.Contains(string(x.Body), "XNightjarY") || !strings.Contains(string(x.Body), `"id":"call-1"`) {
		t.Fatal("protocol fields or non-whole words changed")
	}
	if restored := x.RestoreState().RestoreJSON(x.Body); !bytes.Equal(restored, body) {
		t.Fatalf("nested tool JSON did not restore: %s", restored)
	}
}

func TestFullPolicyOffDoesNoRuntimeWork(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model.Enabled = true // A prepared but disabled policy must remain inert.
	svc := configuredService(t, cfg)
	dir := t.TempDir()
	svc.SetDirectory(dir)
	svc.runtime.find = func(context.Context, string, []string) ([][]redact.Found, error) {
		t.Fatal("disabled privacy invoked a model")
		return nil, nil
	}
	for _, contentType := range []string{"application/json", "binary/unknown", "invalid;="} {
		body := []byte("not JSON " + testKey)
		if x, rejection := svc.Prepare(context.Background(), 1, body, body, contentType); x != nil || rejection != nil {
			t.Fatal("disabled policy inspected or rejected the body")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 || len(svc.runtime.scopes) != 0 {
		t.Fatal("disabled privacy created files or mapping state")
	}
}

func TestFullPolicyModelFailureAndThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Personal, cfg.Model.Enabled = true, true, true
	cfg.Model.Score = .9
	svc := configuredService(t, cfg)
	body := []byte(`{"messages":[{"role":"user","content":"My name is Emilia Brooks."}]}`)
	svc.runtime.find = func(context.Context, string, []string) ([][]redact.Found, error) {
		return nil, errors.New("unavailable")
	}
	x, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection == nil || rejection.HTTPStatusCode != 503 || !x.failed {
		t.Fatal("model failure silently sent content with only rules")
	}
	svc.runtime.find = func(_ context.Context, _ string, texts []string) ([][]redact.Found, error) {
		out := make([][]redact.Found, len(texts))
		for i, text := range texts {
			if start := strings.Index(text, "Emilia Brooks"); start >= 0 {
				out[i] = []redact.Found{{Start: start, End: start + len("Emilia Brooks"), Label: "private_person", Score: .85}}
			}
		}
		return out, nil
	}
	x, rejection = svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil || !bytes.Equal(x.Body, body) {
		t.Fatal("model threshold ignored")
	}
	cfg.Model.Score = .8
	svc.SetConfiguration(cfg)
	x, rejection = svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil || strings.Contains(string(x.Body), "Emilia Brooks") || !bytes.Equal(x.RestoreState().RestoreJSON(x.Body), body) {
		t.Fatal("model finding was not masked/restored after threshold change")
	}
}

func TestFullPolicyProviderPlanAndMultipart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Model.Enabled = true, true
	cfg.BypassProviders = []uint64{10, 11}
	svc := configuredService(t, cfg)
	svc.runtime.find = func(context.Context, string, []string) ([][]redact.Found, error) {
		t.Fatal("exempt plan invoked the model")
		return nil, nil
	}
	body := []byte(`{"messages":[{"role":"user","content":"` + testKey + `"}]}`)
	x, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil || x.ResolvePlan([]uint64{10, 11}) != nil || !x.Bypassed || !bytes.Equal(x.Body, body) {
		t.Fatal("fully exempt plan was not bypassed")
	}
	cfg.Model.Enabled = false
	svc.SetConfiguration(cfg)
	x, _ = svc.Prepare(context.Background(), 1, body, body, "application/json")
	if x.ResolvePlan([]uint64{10, 12}) != nil || x.Bypassed || strings.Contains(string(x.Body), testKey) {
		t.Fatal("mixed fallback plan bypassed privacy")
	}
	cfg.BypassProviders = nil
	svc.SetConfiguration(cfg)
	var buffer bytes.Buffer
	w := multipart.NewWriter(&buffer)
	w.WriteField("prompt", "keep "+testKey+" private")
	file, _ := w.CreateFormFile("image", "sample.bin")
	binary := []byte("\x00\xff" + testKey + "\x00")
	file.Write(binary)
	w.Close()
	x, rejection = svc.Prepare(context.Background(), 1, buffer.Bytes(), buffer.Bytes(), w.FormDataContentType())
	if rejection != nil || !bytes.Contains(x.Body, binary) {
		t.Fatal("multipart file bytes changed")
	}
	reader := multipart.NewReader(bytes.NewReader(x.Body), w.Boundary())
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	text := new(bytes.Buffer)
	text.ReadFrom(part)
	if strings.Contains(text.String(), testKey) {
		t.Fatal("multipart prompt was not masked")
	}
}

func TestFullPolicyNativeDocumentAndFunctionResponse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	svc := configuredService(t, cfg)
	for _, body := range []string{
		`{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"text","media_type":"text/plain","data":"` + testKey + `"}}]}]}`,
		`{"contents":[{"parts":[{"functionResponse":{"name":"tool","response":{"id":"` + testKey + `","password":"tiny"}}}]}]}`,
	} {
		e, rejection := svc.Prepare(context.Background(), 1, []byte(body), []byte(body), "application/json")
		if rejection != nil || strings.Contains(string(e.Body), testKey) || strings.Contains(string(e.Body), "tiny") {
			t.Fatalf("native private content was not inspected: %s %+v", e.Body, rejection)
		}
		if string(e.RestoreState().RestoreJSON(e.Body)) != body {
			t.Fatal("native content did not restore")
		}
	}
}

type unavailablePrivacySettings struct{}

func (unavailablePrivacySettings) Get(string) (string, error) {
	return "", errors.New("storage unavailable")
}
func (unavailablePrivacySettings) Set(string, string) error { return errors.New("storage unavailable") }
func TestPrivacyUnavailablePolicyOmitsUninspectedArchives(t *testing.T) {
	for _, svc := range []*Service{NewService(unavailablePrivacySettings{}), NewService(nil)} {
		body := TextRequest("Nightjar person@corp-real.com")
		e, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
		if rejection == nil || rejection.HTTPStatusCode != 503 || e == nil {
			t.Fatal("policy failure did not fail closed")
		}
		if text := e.Archive(string(body)); strings.Contains(text, "Nightjar") || strings.Contains(text, "person@corp-real.com") {
			t.Fatal("uninspected original reached archive")
		}
		e.BeginRecord(&domain.ProxyRequest{RequestID: "failed"})
	}
	svc := NewService(unavailablePrivacySettings{})
	if filter := svc.EarlyArchiveFilter(1); filter == nil || strings.Contains(filter("Nightjar"), "Nightjar") {
		t.Fatal("early rejection retained uninspected content")
	}
}
func TestExplicitModelCancellationDoesNotSilentlyForward(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Personal = true
	cfg.Model.Enabled = true
	svc := configuredService(t, cfg)
	svc.runtime.find = func(ctx context.Context, _ string, _ []string) ([][]redact.Found, error) { return nil, ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := TextRequest("customer personal content")
	e, rejection := svc.Prepare(ctx, 1, body, body, "application/json")
	if rejection == nil || rejection.HTTPStatusCode != 503 || !e.failed {
		t.Fatal("canceled model request was treated as successful inspection")
	}
}

func TestConvertedPayloadsKeepOccurrenceCountsWithoutRecounting(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{"Nightjar", "Cobalt", "Cedar"}
	svc := configuredService(t, cfg)
	original := TextRequest("Nightjar Nightjar")
	converted := TextRequest("Nightjar Nightjar Cobalt Cobalt")
	e, rejection := svc.Prepare(context.Background(), 1, converted, original, "application/json")
	if rejection != nil {
		t.Fatal(rejection)
	}
	outbound := TextRequest("Nightjar Cobalt Cedar")
	for i := 0; i < 2; i++ {
		masked, rejection := e.Outbound(outbound)
		if rejection != nil || string(e.RestoreState().RestoreJSON(masked)) != string(outbound) {
			t.Fatal("converted payload did not roundtrip", rejection)
		}
	}
	counts := map[string]int{}
	for _, hit := range e.Hits {
		counts[e.pairs[hit.Placeholder]]++
	}
	if counts["Nightjar"] != 2 || counts["Cobalt"] != 2 || counts["Cedar"] != 1 || len(e.Hits) != 5 {
		t.Fatalf("repeated content lost or double-counted: %+v", counts)
	}
}

func TestOutboundCannotSkipPrivacyDecision(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.BypassProviders = []uint64{10}
	svc := configuredService(t, cfg)
	body := TextRequest(testKey)
	e, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil {
		t.Fatal(rejection)
	}
	if wire, rejection := e.Outbound(body); rejection == nil || rejection.HTTPStatusCode != 503 || len(wire) != 0 {
		t.Fatal("outbound forwarded before the exemption plan was resolved")
	}
	if rejection := e.ResolvePlan([]uint64{11}); rejection != nil {
		t.Fatal(rejection)
	}
	if wire, rejection := e.Outbound(body); rejection != nil || bytes.Contains(wire, []byte(testKey)) {
		t.Fatal("resolved mixed plan did not protect outbound content")
	}
}
