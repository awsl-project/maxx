package bedrock

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestAdaptThinkingForModel(t *testing.T) {
	cases := []struct {
		name      string
		shortName string
		input     string
		// Expected post-rewrite fields. Empty string = not asserted.
		wantType   string
		wantBudget bool // expected presence of thinking.budget_tokens
		wantEffort string
	}{
		{
			name:       "opus-4-7 rewrites enabled to adaptive",
			shortName:  "claude-opus-4-7",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000}}`,
			wantType:   "adaptive",
			wantBudget: false,
			wantEffort: "medium",
		},
		{
			name:       "opus-4-7 maps large budget to high effort",
			shortName:  "claude-opus-4-7",
			input:      `{"thinking":{"type":"enabled","budget_tokens":40000}}`,
			wantType:   "adaptive",
			wantEffort: "high",
		},
		{
			name:       "opus-4-7 maps small budget to low effort",
			shortName:  "claude-opus-4-7",
			input:      `{"thinking":{"type":"enabled","budget_tokens":2000}}`,
			wantType:   "adaptive",
			wantEffort: "low",
		},
		{
			name:       "opus-4-7 leaves adaptive untouched",
			shortName:  "claude-opus-4-7",
			input:      `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`,
			wantType:   "adaptive",
			wantBudget: false,
			wantEffort: "medium",
		},
		{
			name:      "opus-4-7 leaves body without thinking untouched",
			shortName: "claude-opus-4-7",
			input:     `{"max_tokens":1024}`,
			wantType:  "",
		},
		{
			name:       "opus-4-5 does not rewrite (classic-only model)",
			shortName:  "claude-opus-4-5",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000}}`,
			wantType:   "enabled",
			wantBudget: true,
			wantEffort: "",
		},
		{
			name:       "opus-4-6 does not rewrite (both supported, leave as-is)",
			shortName:  "claude-opus-4-6",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000}}`,
			wantType:   "enabled",
			wantBudget: true,
			wantEffort: "",
		},
		{
			name:       "opus-4-7 preserves caller-set effort",
			shortName:  "claude-opus-4-7",
			input:      `{"thinking":{"type":"enabled","budget_tokens":1000},"output_config":{"effort":"high"}}`,
			wantType:   "adaptive",
			wantEffort: "high",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AdaptThinkingForModel([]byte(c.input), c.shortName)
			if c.wantType != "" {
				if gt := gjson.GetBytes(got, "thinking.type").String(); gt != c.wantType {
					t.Errorf("thinking.type = %q; want %q (body=%s)", gt, c.wantType, string(got))
				}
			}
			if c.wantEffort != "" {
				if ge := gjson.GetBytes(got, "output_config.effort").String(); ge != c.wantEffort {
					t.Errorf("output_config.effort = %q; want %q (body=%s)", ge, c.wantEffort, string(got))
				}
			}
			budgetExists := gjson.GetBytes(got, "thinking.budget_tokens").Exists()
			if budgetExists != c.wantBudget {
				t.Errorf("budget_tokens exists = %v; want %v (body=%s)", budgetExists, c.wantBudget, string(got))
			}
		})
	}
}

func TestAdaptThinkingForModelStripsSamplingParams(t *testing.T) {
	cases := []struct {
		name      string
		shortName string
		input     string
		// true => the field must be stripped from the output.
		wantStripTemperature bool
		wantStripTopP        bool
		wantStripTopK        bool
	}{
		{
			name:                 "opus-4-7 strips sampling params even without thinking",
			shortName:            "claude-opus-4-7",
			input:                `{"max_tokens":1024,"temperature":0.7,"top_p":0.9,"top_k":40}`,
			wantStripTemperature: true,
			wantStripTopP:        true,
			wantStripTopK:        true,
		},
		{
			name:                 "opus-4-7 strips sampling params when thinking enabled",
			shortName:            "claude-opus-4-7",
			input:                `{"thinking":{"type":"enabled","budget_tokens":4000},"temperature":0.5,"top_p":0.8,"top_k":20}`,
			wantStripTemperature: true,
			wantStripTopP:        true,
			wantStripTopK:        true,
		},
		{
			name:                 "opus-4-5 preserves sampling params (classic-only model)",
			shortName:            "claude-opus-4-5",
			input:                `{"temperature":0.7,"top_p":0.9,"top_k":40}`,
			wantStripTemperature: false,
			wantStripTopP:        false,
			wantStripTopK:        false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AdaptThinkingForModel([]byte(c.input), c.shortName)
			check := func(field string, wantStripped bool) {
				exists := gjson.GetBytes(got, field).Exists()
				if wantStripped && exists {
					t.Errorf("%s should be stripped, still present (body=%s)", field, string(got))
				}
				if !wantStripped && !exists {
					t.Errorf("%s should be preserved, was stripped (body=%s)", field, string(got))
				}
			}
			check("temperature", c.wantStripTemperature)
			check("top_p", c.wantStripTopP)
			check("top_k", c.wantStripTopK)
		})
	}
}

func TestRewriteClassicThinkingToAdaptive(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantEffort string
	}{
		{
			name:       "maps medium budget",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000}}`,
			wantEffort: "medium",
		},
		{
			name:       "preserves explicit effort",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000},"output_config":{"effort":"high"}}`,
			wantEffort: "high",
		},
		{
			name:       "maps missing budget to low",
			input:      `{"thinking":{"type":"enabled"}}`,
			wantEffort: "low",
		},
		{
			name:       "drops extra thinking fields",
			input:      `{"thinking":{"type":"enabled","budget_tokens":16000,"foo":"bar","nested":{"x":1}}}`,
			wantEffort: "medium",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RewriteClassicThinkingToAdaptive([]byte(c.input))
			if gt := gjson.GetBytes(got, "thinking.type").String(); gt != "adaptive" {
				t.Fatalf("thinking.type = %q; want adaptive (body=%s)", gt, string(got))
			}
			if gjson.GetBytes(got, "thinking.budget_tokens").Exists() {
				t.Fatalf("thinking.budget_tokens should be removed (body=%s)", string(got))
			}
			if ge := gjson.GetBytes(got, "output_config.effort").String(); ge != c.wantEffort {
				t.Fatalf("output_config.effort = %q; want %q (body=%s)", ge, c.wantEffort, string(got))
			}
			if thinkingKeys := len(gjson.GetBytes(got, "thinking").Map()); thinkingKeys != 1 {
				t.Fatalf("thinking should contain only type, got %d keys (body=%s)", thinkingKeys, string(got))
			}
		})
	}
}

func TestIsClassicThinkingRejectedError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "reported bedrock validation",
			body: `{"message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}`,
			want: true,
		},
		{
			name: "generic not supported wording",
			body: `{"message":"thinking.type=enabled is not supported; use adaptive thinking"}`,
			want: true,
		},
		{
			name: "unrelated thinking budget error",
			body: `{"message":"thinking.budget_tokens must be less than max_tokens"}`,
			want: false,
		},
		{
			name: "sampling rejection",
			body: `{"message":"temperature may only be set to 1 when thinking is enabled"}`,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsClassicThinkingRejectedError([]byte(c.body)); got != c.want {
				t.Fatalf("IsClassicThinkingRejectedError = %v; want %v", got, c.want)
			}
		})
	}
}
