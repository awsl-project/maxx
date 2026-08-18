package converter

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
)

// Codex declares file-editing tools such as apply_patch as FREEFORM tools, i.e.
// Responses `{"type":"custom","name":"apply_patch","format":{...}}` whose call
// carries a raw text `input` (never JSON). OpenAI Chat Completions (and the
// OpenRouter bridge on top of it) has no custom/freeform tool shape — a request
// carrying `{"type":"custom"}` is rejected — so the Codex→OpenAI bridge used to
// drop every non-function tool. Dropping apply_patch left the model with no way
// to edit files, which surfaced as "the model stopped calling tools / got dumb"
// on the OpenRouter fallback path while native Codex providers were unaffected.
//
// The bridge instead represents a freeform tool as an ordinary function tool
// taking a single required string parameter `input`, and translates the call
// back and forth: the freeform `input` becomes `{"input":"<raw>"}` function
// arguments on the wire and is unwrapped to a Codex `custom_tool_call` (raw
// `input`) on the way back. See codexToOpenAIRequest.Transform and
// openaiToCodexResponse.

// freeformInputProperty is the single function parameter that carries a freeform
// tool's raw text input when a Codex custom tool is represented as an OpenAI
// function tool.
const freeformInputProperty = "input"

// codexFreeformFunctionParameters is the JSON Schema for the synthetic function
// wrapper of a Codex freeform/custom tool.
func codexFreeformFunctionParameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			freeformInputProperty: map[string]interface{}{
				"type":        "string",
				"description": "The complete raw text input for this tool, passed through verbatim (do not wrap it in JSON).",
			},
		},
		"required": []interface{}{freeformInputProperty},
	}
}

// isCodexFreeformToolType reports whether a Codex tool type denotes a freeform
// (custom) tool that must be wrapped as a function for OpenAI Chat.
func isCodexFreeformToolType(toolType string) bool {
	switch strings.ToLower(strings.TrimSpace(toolType)) {
	case "custom", "freeform":
		return true
	default:
		return false
	}
}

// codexCustomToolNames returns the set of tool names the original Codex request
// declared as freeform/custom, so the response side knows which upstream
// function tool_calls to re-emit as Codex custom_tool_call items.
func codexCustomToolNames(requestRaw []byte) map[string]bool {
	names := map[string]bool{}
	tools := gjson.GetBytes(requestRaw, "tools")
	if !tools.IsArray() {
		return names
	}
	for _, t := range tools.Array() {
		if !isCodexFreeformToolType(t.Get("type").String()) {
			continue
		}
		if name := t.Get("name").String(); name != "" {
			names[name] = true
		}
	}
	return names
}

// wrapFreeformInputAsArgs turns a freeform tool's raw text input into the
// `{"input":"<raw>"}` JSON arguments an OpenAI Chat function call carries.
func wrapFreeformInputAsArgs(input string) string {
	encoded, err := json.Marshal(map[string]string{freeformInputProperty: input})
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

// unwrapFreeformArgsToInput extracts the raw freeform text from function-call
// arguments produced for a wrapped freeform tool. It returns the `input` field
// when present; otherwise it falls back to the arguments verbatim so a
// mis-shaped payload still round-trips something usable.
func unwrapFreeformArgsToInput(args string) string {
	if v := gjson.Get(args, freeformInputProperty); v.Exists() {
		return v.String()
	}
	return args
}
