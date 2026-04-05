package handler

import "testing"

func TestParseProviderPath(t *testing.T) {
	h := &ProviderProxyHandler{}
	providerID, apiPath, ok := h.parseProviderPath("/provider/1/v1/chat/completions")
	if !ok {
		t.Fatal("expected provider path to parse")
	}
	if providerID != "1" {
		t.Fatalf("providerID = %q, want 1", providerID)
	}
	if apiPath != "/v1/chat/completions" {
		t.Fatalf("apiPath = %q, want /v1/chat/completions", apiPath)
	}
}

func TestParseProviderPath_TrimsProviderID(t *testing.T) {
	h := &ProviderProxyHandler{}
	providerID, apiPath, ok := h.parseProviderPath("/provider/ 1 /v1/messages")
	if !ok {
		t.Fatal("expected provider path to parse")
	}
	if providerID != "1" {
		t.Fatalf("providerID = %q, want 1", providerID)
	}
	if apiPath != "/v1/messages" {
		t.Fatalf("apiPath = %q, want /v1/messages", apiPath)
	}
}

func TestIsValidProviderAPIPath_AllowsExactAndSubpathsOnly(t *testing.T) {
	valid := []string{
		"/v1/messages",
		"/v1/messages/stream",
		"/v1/chat/completions",
		"/v1/chat/completions/extra",
		"/responses",
		"/responses/items",
		"/v1/responses",
		"/v1/responses/abc",
		"/v1/models",
		"/v1/models/list",
		"/v1beta/models",
		"/v1beta/models/gemini-2.5-pro",
	}
	for _, path := range valid {
		if !isValidProviderAPIPath(path) {
			t.Fatalf("expected %q to be valid", path)
		}
	}

	invalid := []string{
		"/v1/messages-debug",
		"/v1/chat/completionsXYZ",
		"/responses123",
		"/v1/responsesXYZ",
		"/v1/models-debug",
		"/v1beta/modelsX",
	}
	for _, path := range invalid {
		if isValidProviderAPIPath(path) {
			t.Fatalf("expected %q to be invalid", path)
		}
	}
}

func TestIsProviderProxyPath(t *testing.T) {
	if !isProviderProxyPath("/provider/1/v1/messages") {
		t.Fatal("expected provider path to be detected")
	}
	if isProviderProxyPath("/project/demo/v1/messages") {
		t.Fatal("did not expect project path to be detected as provider path")
	}
	if isProviderProxyPath("/providers") {
		t.Fatal("did not expect regular web route to be detected")
	}
}
