package antigravity

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestApplyAntigravityRequestTuning(t *testing.T) {
	input := `{
  "request": {
    "systemInstruction": {
      "parts": [{"text":"original"}]
    },
    "tools": [{
      "functionDeclarations": [{
        "name": "t1",
        "parametersJsonSchema": {"type":"object","properties":{"x":{"type":"string"}}}
      }]
    }]
  },
  "model": "claude-sonnet-4-5"
}`
	out := applyAntigravityRequestTuning([]byte(input), "claude-sonnet-4-5")

	if !gjson.GetBytes(out, "request.systemInstruction.role").Exists() {
		t.Fatalf("expected systemInstruction.role to be set")
	}
	if gjson.GetBytes(out, "request.systemInstruction.parts.0.text").String() == "" {
		t.Fatalf("expected systemInstruction parts[0].text to be injected")
	}
	if gjson.GetBytes(out, "request.systemInstruction.parts.1.text").String() == "" {
		t.Fatalf("expected systemInstruction parts[1].text to be injected")
	}
	if gjson.GetBytes(out, "request.toolConfig.functionCallingConfig.mode").String() != "VALIDATED" {
		t.Fatalf("expected toolConfig.functionCallingConfig.mode=VALIDATED")
	}
	if gjson.GetBytes(out, "request.tools.0.functionDeclarations.0.parametersJsonSchema").Exists() {
		t.Fatalf("expected parametersJsonSchema to be renamed")
	}
	if !gjson.GetBytes(out, "request.tools.0.functionDeclarations.0.parameters").Exists() {
		t.Fatalf("expected parameters to exist after rename")
	}
}

func TestExtractModelVersionFromSSELine(t *testing.T) {
	if got := extractModelVersionFromSSELine("data: {\"modelVersion\":\"gemini-2.5-pro\"}\n"); got != "gemini-2.5-pro" {
		t.Fatalf("expected direct modelVersion, got %q", got)
	}
	if got := extractModelVersionFromSSELine("data: {\"response\":{\"modelVersion\":\"gemini-2.5-flash\"}}\n"); got != "gemini-2.5-flash" {
		t.Fatalf("expected wrapped modelVersion, got %q", got)
	}
	if got := extractModelVersionFromSSELine("event: message\n"); got != "" {
		t.Fatalf("expected empty for non-data line, got %q", got)
	}
	if got := extractModelVersionFromSSELine("data: [DONE]\n"); got != "" {
		t.Fatalf("expected empty for done line, got %q", got)
	}
	if got := extractModelVersionFromSSELine("data: {invalid}\n"); got != "" {
		t.Fatalf("expected empty for invalid json, got %q", got)
	}
	if got := extractModelVersionFromSSELine("data: {\"other\":\"value\"}\n"); got != "" {
		t.Fatalf("expected empty when modelVersion missing, got %q", got)
	}
}
