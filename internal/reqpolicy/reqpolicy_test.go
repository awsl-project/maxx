package reqpolicy

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func mustPolicy(max, def string) *domain.ReasoningPolicy {
	return &domain.ReasoningPolicy{MaxEffort: max, DefaultEffort: def}
}

// effortAt reads the codec path for a protocol back out of a body, lower-cased,
// so assertions are protocol-agnostic (Gemini writes upper-case).
func effortAt(t *testing.T, body []byte, protocol domain.ClientType) string {
	t.Helper()
	c := codecFor(protocol)
	if c == nil {
		t.Fatalf("no codec for %s", protocol)
	}
	return gjson.GetBytes(body, c.path).String()
}

func TestApply_ClampsAboveCeiling(t *testing.T) {
	eff := Resolve(nil, mustPolicy("medium", ""), "")
	out := Apply([]byte(`{"model":"m","reasoning_effort":"high"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, out, domain.ClientTypeOpenAI); got != "medium" {
		t.Fatalf("reasoning_effort = %q, want medium", got)
	}
}

func TestApply_LeavesBelowCeilingUntouched(t *testing.T) {
	eff := Resolve(nil, mustPolicy("high", ""), "")
	out := Apply([]byte(`{"reasoning_effort":"low"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, out, domain.ClientTypeOpenAI); got != "low" {
		t.Fatalf("reasoning_effort = %q, want low (unchanged)", got)
	}
}

func TestApply_AutoClampsToCeiling(t *testing.T) {
	// auto means "provider decides" and can resolve to high upstream; a ceiling
	// must bound it, else the cap leaks.
	eff := Resolve(nil, mustPolicy("medium", ""), "")
	out := Apply([]byte(`{"reasoning_effort":"auto"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, out, domain.ClientTypeOpenAI); got != "medium" {
		t.Fatalf("auto reasoning_effort = %q, want clamped to medium", got)
	}
}

func TestApply_AutoPassesThroughWithoutCeiling(t *testing.T) {
	eff := Resolve(nil, mustPolicy("", "low"), "") // default only, no ceiling
	out := Apply([]byte(`{"reasoning_effort":"auto"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, out, domain.ClientTypeOpenAI); got != "auto" {
		t.Fatalf("auto reasoning_effort = %q, want left as auto", got)
	}
}

func TestApply_UnknownVocabularyClampedWhenCapped(t *testing.T) {
	// A ceiling must hold even against a value we can't rank.
	eff := Resolve(nil, mustPolicy("medium", ""), "")
	out := Apply([]byte(`{"reasoning_effort":"ultra"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, out, domain.ClientTypeOpenAI); got != "medium" {
		t.Fatalf("unknown reasoning_effort = %q, want clamped to medium", got)
	}
}

func TestApply_ExtendedEffortsAreOrderedAndWritable(t *testing.T) {
	clamped := Apply(
		[]byte(`{"reasoning_effort":"max"}`),
		domain.ClientTypeOpenAI,
		Resolve(nil, mustPolicy("xhigh", ""), ""),
	)
	if got := effortAt(t, clamped, domain.ClientTypeOpenAI); got != "xhigh" {
		t.Fatalf("max above xhigh ceiling = %q, want xhigh", got)
	}

	filled := Apply(
		[]byte(`{"model":"m"}`),
		domain.ClientTypeOpenAI,
		Resolve(nil, mustPolicy("", "max"), ""),
	)
	if got := effortAt(t, filled, domain.ClientTypeOpenAI); got != "max" {
		t.Fatalf("default effort = %q, want max", got)
	}
}

func TestApply_DefaultFillsOnlyWhenAbsent(t *testing.T) {
	eff := Resolve(nil, mustPolicy("high", "low"), "")

	filled := Apply([]byte(`{"model":"m"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, filled, domain.ClientTypeOpenAI); got != "low" {
		t.Fatalf("absent effort = %q, want default low", got)
	}

	kept := Apply([]byte(`{"reasoning_effort":"medium"}`), domain.ClientTypeOpenAI, eff)
	if got := effortAt(t, kept, domain.ClientTypeOpenAI); got != "medium" {
		t.Fatalf("present effort = %q, want kept medium (default must not override)", got)
	}
}

func TestApply_Idempotent(t *testing.T) {
	eff := Resolve(nil, mustPolicy("medium", ""), "")
	once := Apply([]byte(`{"reasoning_effort":"high"}`), domain.ClientTypeOpenAI, eff)
	twice := Apply(once, domain.ClientTypeOpenAI, eff)
	if string(once) != string(twice) {
		t.Fatalf("not idempotent: once=%s twice=%s", once, twice)
	}
}

func TestApply_CodexNestedPath(t *testing.T) {
	eff := Resolve(nil, mustPolicy("low", ""), "")
	out := Apply([]byte(`{"reasoning":{"effort":"high"}}`), domain.ClientTypeCodex, eff)
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "low" {
		t.Fatalf("reasoning.effort = %q, want low", got)
	}
}

func TestApply_GeminiUpperCaseWire(t *testing.T) {
	eff := Resolve(nil, mustPolicy("medium", ""), "")
	body := []byte(`{"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}`)
	out := Apply(body, domain.ClientTypeGemini, eff)
	if got := gjson.GetBytes(out, "generationConfig.thinkingConfig.thinkingLevel").String(); got != "MEDIUM" {
		t.Fatalf("thinkingLevel = %q, want MEDIUM (upper-case wire)", got)
	}
}

func TestApply_GeminiCapsExtendedWireToHigh(t *testing.T) {
	eff := Resolve(nil, mustPolicy("", "max"), "")
	out := Apply([]byte(`{}`), domain.ClientTypeGemini, eff)
	if got := gjson.GetBytes(out, "generationConfig.thinkingConfig.thinkingLevel").String(); got != "HIGH" {
		t.Fatalf("thinkingLevel = %q, want HIGH (Gemini wire maximum)", got)
	}
}

func TestValidEffort_AcceptsExtendedEfforts(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", ""} {
		if !ValidEffort(effort) {
			t.Fatalf("ValidEffort(%q) = false, want true", effort)
		}
	}
	if ValidEffort("ultra") {
		t.Fatal("ValidEffort(ultra) = true, want false")
	}
}

func TestApply_ClaudeDefaultWritesThinkingBudget(t *testing.T) {
	eff := Resolve(nil, mustPolicy("", "medium"), "")
	out := Apply([]byte(`{"model":"claude-sonnet","max_tokens":4096}`), domain.ClientTypeClaude, eff)
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q, want enabled; body=%s", got, out)
	}
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 8192 {
		t.Fatalf("thinking.budget_tokens = %d, want 8192; body=%s", got, out)
	}
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 8193 {
		t.Fatalf("max_tokens = %d, want bumped above thinking budget; body=%s", got, out)
	}
}

func TestApply_ClaudeMaxWritesHighestThinkingBudget(t *testing.T) {
	eff := Resolve(nil, mustPolicy("", "max"), "")
	out := Apply([]byte(`{"model":"claude-sonnet","max_tokens":4096}`), domain.ClientTypeClaude, eff)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 64000 {
		t.Fatalf("thinking.budget_tokens = %d, want 64000; body=%s", got, out)
	}
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 64001 {
		t.Fatalf("max_tokens = %d, want bumped above max thinking budget; body=%s", got, out)
	}
}

func TestApply_ClaudeClampsExistingThinkingBudget(t *testing.T) {
	eff := Resolve(nil, mustPolicy("low", ""), "")
	out := Apply([]byte(`{"thinking":{"type":"enabled","budget_tokens":20000},"max_tokens":30000}`), domain.ClientTypeClaude, eff)
	if got := gjson.GetBytes(out, "thinking.budget_tokens").Int(); got != 1024 {
		t.Fatalf("thinking.budget_tokens = %d, want 1024; body=%s", got, out)
	}
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 30000 {
		t.Fatalf("max_tokens = %d, want unchanged above budget; body=%s", got, out)
	}
}

func TestApply_ClaudeNoneDisablesThinking(t *testing.T) {
	eff := Resolve(nil, mustPolicy("none", ""), "")
	out := Apply([]byte(`{"thinking":{"type":"enabled","budget_tokens":2048}}`), domain.ClientTypeClaude, eff)
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "disabled" {
		t.Fatalf("thinking.type = %q, want disabled; body=%s", got, out)
	}
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("thinking.budget_tokens should be removed; body=%s", out)
	}
}

func TestApply_UnsupportedProtocolNoOp(t *testing.T) {
	eff := Resolve(nil, mustPolicy("low", "low"), "")
	in := []byte(`{"reasoning_effort":"high"}`)
	out := Apply(in, domain.ClientType("future"), eff)
	if string(out) != string(in) {
		t.Fatalf("unsupported protocol mutated body: %s", out)
	}
}

func TestApply_ZeroPolicyNoOp(t *testing.T) {
	in := []byte(`{"reasoning_effort":"high"}`)
	out := Apply(in, domain.ClientTypeOpenAI, Effective{})
	if string(out) != string(in) {
		t.Fatalf("zero policy mutated body: %s", out)
	}
}

func TestResolve_CeilingComposesByMin(t *testing.T) {
	// global high, provider medium -> effective medium (lower wins).
	eff := Resolve(mustPolicy("high", ""), mustPolicy("medium", ""), "")
	if !eff.hasMax || eff.maxRank != rankMedium {
		t.Fatalf("effective max = %+v, want medium", eff)
	}
	// global medium, provider unset -> medium.
	eff = Resolve(mustPolicy("medium", ""), nil, "")
	if !eff.hasMax || eff.maxRank != rankMedium {
		t.Fatalf("effective max = %+v, want medium from global", eff)
	}
}

func TestResolve_DefaultMostSpecificWins(t *testing.T) {
	eff := Resolve(mustPolicy("", "low"), mustPolicy("", "high"), "medium")
	if !eff.hasDefault || eff.defaultRank != rankHigh {
		t.Fatalf("default = %+v, want provider's high", eff)
	}
	// provider unset -> global.
	eff = Resolve(mustPolicy("", "low"), nil, "medium")
	if !eff.hasDefault || eff.defaultRank != rankLow {
		t.Fatalf("default = %+v, want global low", eff)
	}
	// only legacy set -> legacy demoted to default.
	eff = Resolve(nil, nil, "medium")
	if !eff.hasDefault || eff.defaultRank != rankMedium {
		t.Fatalf("default = %+v, want legacy medium", eff)
	}
}

func TestResolve_LegacyIsDefaultNotForce(t *testing.T) {
	// Legacy Codex effort must NOT override an explicit client value (it is a
	// default now, not a force). With legacy=high and client=low, low stays.
	eff := Resolve(nil, nil, "high")
	out := Apply([]byte(`{"reasoning":{"effort":"low"}}`), domain.ClientTypeCodex, eff)
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "low" {
		t.Fatalf("reasoning.effort = %q, want low (legacy default must not force)", got)
	}
}
