package handler

import "strings"

// proxyAPIEndpoint is one public AI API path family accepted under the
// /project/<slug>/ and /provider/<id>/ prefixes.
type proxyAPIEndpoint struct {
	path    string // canonical path, no trailing slash
	subtree bool   // also accept "<path>/..." (but never "<path>X")
}

// proxyAPIEndpoints is the single source of truth for which apiPaths the
// project- and provider-scoped proxies accept. isValidAPIPath (project) and
// isValidProviderAPIPath (provider) both derive from this table — they used to be
// two hand-maintained copies that had already drifted (project matched with a
// loose HasPrefix that accepted "/v1/messages-debug"; provider used the stricter
// exact-or-subtree form), so adding an endpoint meant editing two lists and
// hoping they stayed in sync.
//
// Keep this aligned with the root routes in core.RegisterProxyRoutes, which is
// the direct-access counterpart. That table additionally exposes bare aliases
// (e.g. /chat/completions, /images) that are only reachable directly and are
// never valid as a project/provider apiPath, plus the Gemini split where
// /v1beta/models (exact) is the model-list handler while /v1beta/models/<m>
// (subtree) is a generation endpoint — both are accepted here as one family.
//
// Image endpoints are exact-only (subtree:false): only the endpoints
// core.RegisterProxyRoutes registers pass, so /v1/images/ and
// /v1/images/variations do not leak the project/provider contract wider than the
// root.
var proxyAPIEndpoints = []proxyAPIEndpoint{
	{path: "/v1/messages", subtree: true},            // Claude API
	{path: "/v1/chat/completions", subtree: true},    // OpenAI API
	{path: "/v1/images/generations", subtree: false}, // OpenAI Images API
	{path: "/v1/images/edits", subtree: false},       // OpenAI Images API
	{path: "/v1/images", subtree: false},             // OpenRouter unified image API
	{path: "/responses", subtree: true},              // Codex API
	{path: "/v1/responses", subtree: true},           // Codex API
	{path: "/v1/models", subtree: true},              // Model list API
	{path: "/v1beta/models", subtree: true},          // Gemini API (list + generation)
}

// isValidProxyAPIPath reports whether path is a known proxy API endpoint, using
// exact-or-subtree matching so "/v1/messages-debug" is rejected while
// "/v1/messages" and "/v1/messages/stream" pass.
func isValidProxyAPIPath(path string) bool {
	for _, e := range proxyAPIEndpoints {
		if path == e.path {
			return true
		}
		if e.subtree && strings.HasPrefix(path, e.path+"/") {
			return true
		}
	}
	return false
}
