package payloadoverride

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func rawJSON(value string) json.RawMessage {
	return json.RawMessage(value)
}

func TestParseRules(t *testing.T) {
	rules, err := ParseRules(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {"instructions": "hello"}
		},
		{
			"models": [{"name": "  *o3*  "}],
			"params": {"service_tier": "priority"}
		}
	]`)
	if err != nil {
		t.Fatalf("ParseRules returned error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if got := rules[0].Models[0].Protocol; got != "codex" {
		t.Fatalf("expected normalized protocol codex, got %q", got)
	}
	if got := rules[1].Models[0].Name; got != "*o3*" {
		t.Fatalf("expected trimmed selector name, got %q", got)
	}
	if got := rules[1].Models[0].Protocol; got != "codex" {
		t.Fatalf("expected omitted protocol to normalize to codex, got %q", got)
	}
}

func TestParseRulesSkipsReservedPaths(t *testing.T) {
	rules, err := ParseRules(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {
				"model": "other",
				"stream": false,
				"instructions": "hello",
				"reasoning.effort": "high"
			}
		}
	]`)
	if err != nil {
		t.Fatalf("ParseRules returned error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if _, ok := rules[0].Params["model"]; ok {
		t.Fatalf("expected model path to be filtered out")
	}
	if _, ok := rules[0].Params["stream"]; ok {
		t.Fatalf("expected stream path to be filtered out")
	}
	if got := string(rules[0].Params["instructions"]); got != `"hello"` {
		t.Fatalf("expected instructions to remain, got %#v", got)
	}
}

func TestParseRulesInvalidJSON(t *testing.T) {
	if _, err := ParseRules(`{"models":[}`); err == nil {
		t.Fatalf("expected ParseRules to fail for invalid json")
	}
}

func TestValidateRulesJSON(t *testing.T) {
	err := ValidateRulesJSON(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {
				"instructions": "hello",
				"reasoning.effort": "high"
			}
		}
	]`)
	if err != nil {
		t.Fatalf("expected valid rules, got error: %v", err)
	}
}

func TestValidateRulesJSONRejectsEmptySetting(t *testing.T) {
	err := ValidateRulesJSON(`   `)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateRulesJSONRejectsNonArrayTopLevel(t *testing.T) {
	err := ValidateRulesJSON(`null`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateRulesJSONRejectsEmptyParams(t *testing.T) {
	err := ValidateRulesJSON(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {}
		}
	]`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateRulesJSONRejectsReservedPaths(t *testing.T) {
	err := ValidateRulesJSON(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {"stream": false}
		}
	]`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateRulesJSONRejectsUnsupportedProtocol(t *testing.T) {
	err := ValidateRulesJSON(`[
		{
			"models": [{"name": "gpt-5.4", "protocol": "openai"}],
			"params": {"instructions": "hello"}
		}
	]`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestValidateRulesJSONRejectsDuplicateSelectors(t *testing.T) {
	err := ValidateRulesJSON(`[
		{
			"models": [{"name": "gpt-5.4"}],
			"params": {"instructions": "hello"}
		},
		{
			"models": [{"name": "gpt-5.4", "protocol": "codex"}],
			"params": {"instructions": "world"}
		}
	]`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestApplyRules(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.4",
		"instructions":"original",
		"reasoning":{"effort":"low"},
		"metadata":{"flags":["before"],"enabled":false}
	}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
			Params: map[string]json.RawMessage{
				"instructions":      rawJSON(`"overridden"`),
				"reasoning.effort":  rawJSON(`"high"`),
				"metadata.enabled":  rawJSON(`true`),
				"metadata.count":    rawJSON(`2`),
				"metadata.flags":    rawJSON(`["after"]`),
				"metadata.extra.x":  rawJSON(`"y"`),
				"metadata.settings": rawJSON(`{"mode":"strict"}`),
			},
		},
	}

	got := ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "instructions").String(); v != "overridden" {
		t.Fatalf("expected instructions overridden, got %q", v)
	}
	if v := gjson.GetBytes(got, "reasoning.effort").String(); v != "high" {
		t.Fatalf("expected reasoning.effort overridden, got %q", v)
	}
	if !gjson.GetBytes(got, "metadata.enabled").Bool() {
		t.Fatalf("expected metadata.enabled overridden")
	}
	if v := gjson.GetBytes(got, "metadata.count").Int(); v != 2 {
		t.Fatalf("expected metadata.count=2, got %d", v)
	}
	if v := gjson.GetBytes(got, "metadata.flags.0").String(); v != "after" {
		t.Fatalf("expected metadata.flags replaced, got %q", v)
	}
	if v := gjson.GetBytes(got, "metadata.extra.x").String(); v != "y" {
		t.Fatalf("expected nested metadata.extra.x, got %q", v)
	}
	if v := gjson.GetBytes(got, "metadata.settings.mode").String(); v != "strict" {
		t.Fatalf("expected metadata.settings.mode=strict, got %q", v)
	}
}

func TestApplyRulesSkipsReservedPaths(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","stream":true,"instructions":"original"}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
			Params: map[string]json.RawMessage{
				"model":        rawJSON(`"gpt-5.5"`),
				"stream":       rawJSON(`false`),
				"instructions": rawJSON(`"overridden"`),
			},
		},
	}

	got := ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "model").String(); v != "gpt-5.4" {
		t.Fatalf("expected model to remain unchanged, got %q", v)
	}
	if !gjson.GetBytes(got, "stream").Bool() {
		t.Fatalf("expected stream to remain unchanged")
	}
	if v := gjson.GetBytes(got, "instructions").String(); v != "overridden" {
		t.Fatalf("expected instructions overridden, got %q", v)
	}
}

func TestApplyRulesUsesLastMatchingRule(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","instructions":"original"}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.*", Protocol: "codex"}},
			Params: map[string]json.RawMessage{"instructions": rawJSON(`"first"`)},
		},
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
			Params: map[string]json.RawMessage{"instructions": rawJSON(`"second"`)},
		},
	}

	got := ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "instructions").String(); v != "second" {
		t.Fatalf("expected later rule to win, got %q", v)
	}
}

func TestApplyRulesSkipsProtocolMismatch(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","instructions":"original"}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "openai"}},
			Params: map[string]json.RawMessage{"instructions": rawJSON(`"overridden"`)},
		},
	}

	got := ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "instructions").String(); v != "original" {
		t.Fatalf("expected protocol mismatch to skip override, got %q", v)
	}
}

func TestApplyRulesFallsBackToBodyModel(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","instructions":"original"}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
			Params: map[string]json.RawMessage{"instructions": rawJSON(`"overridden"`)},
		},
	}

	got := ApplyRules(body, rules, "codex", "")
	if v := gjson.GetBytes(got, "instructions").String(); v != "overridden" {
		t.Fatalf("expected body model fallback, got %q", v)
	}
}

func TestGetGlobalSettingsCachesUntilInvalidated(t *testing.T) {
	t.Cleanup(func() {
		SetGlobalSettingsGetter(nil)
		InvalidateGlobalSettingsCache()
	})

	loadCount := 0
	SetGlobalSettingsGetter(func() (*GlobalSettings, error) {
		loadCount++
		return &GlobalSettings{
			Rules: []Rule{
				{
					Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
					Params: map[string]json.RawMessage{"instructions": rawJSON(`"hello"`)},
				},
			},
		}, nil
	})

	first := GetGlobalSettings()
	second := GetGlobalSettings()
	if first == nil || second == nil {
		t.Fatalf("expected cached settings to be available")
	}
	if loadCount != 1 {
		t.Fatalf("expected getter to be called once before invalidation, got %d", loadCount)
	}

	InvalidateGlobalSettingsCache()
	third := GetGlobalSettings()
	if third == nil {
		t.Fatalf("expected settings after invalidation")
	}
	if loadCount != 2 {
		t.Fatalf("expected getter to be called again after invalidation, got %d", loadCount)
	}
}

func TestApplyRulesPreservesLargeJSONNumbers(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","metadata":{"large":0}}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
			Params: map[string]json.RawMessage{
				"metadata.large": rawJSON(`9007199254740993`),
			},
		},
	}

	got := ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "metadata.large").Raw; v != "9007199254740993" {
		t.Fatalf("expected large integer precision to be preserved, got %q", v)
	}
}

func TestApplyRulesTreatsOmittedProtocolAsCodex(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","instructions":"original"}`)
	rules := []Rule{
		{
			Models: []ModelSelector{{Name: "gpt-5.4"}},
			Params: map[string]json.RawMessage{"instructions": rawJSON(`"overridden"`)},
		},
	}

	got := ApplyRules(body, rules, "openai", "gpt-5.4")
	if v := gjson.GetBytes(got, "instructions").String(); v != "original" {
		t.Fatalf("expected omitted protocol to default to codex only, got %q", v)
	}

	got = ApplyRules(body, rules, "codex", "gpt-5.4")
	if v := gjson.GetBytes(got, "instructions").String(); v != "overridden" {
		t.Fatalf("expected codex protocol to match omitted protocol, got %q", v)
	}
}

func TestReloadGlobalSettingsDoesNotOverwriteInvalidatedCache(t *testing.T) {
	t.Cleanup(func() {
		SetGlobalSettingsGetter(nil)
		InvalidateGlobalSettingsCache()
	})

	var oldGetterCalls int32
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	SetGlobalSettingsGetter(func() (*GlobalSettings, error) {
		atomic.AddInt32(&oldGetterCalls, 1)
		close(oldStarted)
		<-releaseOld
		return &GlobalSettings{
			Rules: []Rule{
				{
					Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
					Params: map[string]json.RawMessage{"instructions": rawJSON(`"old"`)},
				},
			},
		}, nil
	})

	done := make(chan *GlobalSettings, 1)
	errCh := make(chan error, 1)
	go func() {
		settings, err := ReloadGlobalSettings()
		if err != nil {
			errCh <- err
			return
		}
		done <- settings
	}()

	select {
	case <-oldStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for old getter to start")
	}

	SetGlobalSettingsGetter(func() (*GlobalSettings, error) {
		return &GlobalSettings{
			Rules: []Rule{
				{
					Models: []ModelSelector{{Name: "gpt-5.4", Protocol: "codex"}},
					Params: map[string]json.RawMessage{"instructions": rawJSON(`"new"`)},
				},
			},
		}, nil
	})
	close(releaseOld)

	select {
	case err := <-errCh:
		t.Fatalf("ReloadGlobalSettings returned error: %v", err)
	case settings := <-done:
		if settings == nil {
			t.Fatalf("expected settings to be returned")
		}
		if got := string(settings.Rules[0].Params["instructions"]); got != `"new"` {
			t.Fatalf("expected reloaded settings to use the latest getter, got %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReloadGlobalSettings")
	}

	cached := GetGlobalSettings()
	if cached == nil {
		t.Fatalf("expected cached settings after reload")
	}
	if got := string(cached.Rules[0].Params["instructions"]); got != `"new"` {
		t.Fatalf("expected cache to keep new settings, got %s", got)
	}
}
