package core

import "net/http"

// ProxyRouteHandlers groups the public AI API handlers that must stay in sync
// across CLI, desktop, and test route registration.
type ProxyRouteHandlers struct {
	ProxyHandler         http.Handler
	ModelsHandler        http.Handler
	ProviderProxyHandler http.Handler
}

// RegisterProxyRoutes registers the shared public AI API routes.
func RegisterProxyRoutes(mux *http.ServeMux, handlers ProxyRouteHandlers) {
	if mux == nil {
		return
	}

	if handlers.ProxyHandler != nil {
		// Claude API
		mux.Handle("/v1/messages", handlers.ProxyHandler)
		mux.Handle("/v1/messages/", handlers.ProxyHandler)
		// OpenAI API
		mux.Handle("/v1/chat/completions", handlers.ProxyHandler)
		// OpenAI Images API (gpt-image-* generation + edits)
		mux.Handle("/v1/images/generations", handlers.ProxyHandler)
		mux.Handle("/v1/images/edits", handlers.ProxyHandler)
		// Codex API
		mux.Handle("/responses", handlers.ProxyHandler)
		mux.Handle("/responses/", handlers.ProxyHandler)
		mux.Handle("/v1/responses", handlers.ProxyHandler)
		mux.Handle("/v1/responses/", handlers.ProxyHandler)
		// Gemini API (Google AI Studio style generation endpoints)
		mux.Handle("/v1beta/models/", handlers.ProxyHandler)
	}

	if handlers.ModelsHandler != nil {
		mux.Handle("/v1/models", handlers.ModelsHandler)
		mux.Handle("/v1beta/models", handlers.ModelsHandler)
	}

	if handlers.ProviderProxyHandler != nil {
		mux.Handle("/provider/", handlers.ProviderProxyHandler)
	}
}
