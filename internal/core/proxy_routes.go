package core

import (
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// ProxyRouteHandlers groups the public AI API handlers that must stay in sync
// across CLI, desktop, and test route registration.
type ProxyRouteHandlers struct {
	ProxyHandler         http.Handler
	ModelsHandler        http.Handler
	ProviderProxyHandler http.Handler
	SettingRepo          repository.SystemSettingRepository
}

// RegisterProxyRoutes registers the shared public AI API routes.
func RegisterProxyRoutes(mux *http.ServeMux, handlers ProxyRouteHandlers) {
	if mux == nil {
		return
	}

	if handlers.ProxyHandler != nil {
		claudeMessagesHandler := routeGate(handlers.ProxyHandler, handlers.SettingRepo, domain.SettingKeyProxyRouteClaudeMessagesEnabled)
		openAIChatHandler := routeGate(handlers.ProxyHandler, handlers.SettingRepo, domain.SettingKeyProxyRouteOpenAIChatEnabled)
		responsesHandler := routeGate(handlers.ProxyHandler, handlers.SettingRepo, domain.SettingKeyProxyRouteResponsesEnabled)
		geminiHandler := routeGate(handlers.ProxyHandler, handlers.SettingRepo, domain.SettingKeyProxyRouteGeminiEnabled)

		// Claude API
		mux.Handle("/v1/messages", claudeMessagesHandler)
		mux.Handle("/v1/messages/", claudeMessagesHandler)
		// OpenAI API
		mux.Handle("/v1/chat/completions", openAIChatHandler)
		mux.Handle("/chat/completions", openAIChatHandler)
		// OpenAI Images API (gpt-image-* generation + edits)
		mux.Handle("/v1/images/generations", handlers.ProxyHandler)
		mux.Handle("/v1/images/edits", handlers.ProxyHandler)
		mux.Handle("/images/generations", handlers.ProxyHandler)
		mux.Handle("/images/edits", handlers.ProxyHandler)
		// OpenRouter unified image API (POST /v1/images → data[].b64_json)
		mux.Handle("/v1/images", handlers.ProxyHandler)
		mux.Handle("/images", handlers.ProxyHandler)
		// Async video generation (new-api style): POST submit + GET /{task_id} poll.
		// The trailing-slash subtree pattern matches the /{task_id} poll path.
		mux.Handle("/v1/video/generations", handlers.ProxyHandler)
		mux.Handle("/v1/video/generations/", handlers.ProxyHandler)
		mux.Handle("/video/generations", handlers.ProxyHandler)
		mux.Handle("/video/generations/", handlers.ProxyHandler)
		mux.Handle("/v1/videos", handlers.ProxyHandler)
		mux.Handle("/v1/videos/", handlers.ProxyHandler)
		mux.Handle("/videos", handlers.ProxyHandler)
		mux.Handle("/videos/", handlers.ProxyHandler)
		// Codex API
		mux.Handle("/responses", responsesHandler)
		mux.Handle("/responses/", responsesHandler)
		mux.Handle("/v1/responses", responsesHandler)
		mux.Handle("/v1/responses/", responsesHandler)
		// Gemini API (Google AI Studio style generation endpoints)
		mux.Handle("/v1beta/models/", geminiHandler)
	}

	if handlers.ModelsHandler != nil {
		mux.Handle("/v1/models", handlers.ModelsHandler)
		mux.Handle("/models", handlers.ModelsHandler)
		mux.Handle("/v1beta/models", handlers.ModelsHandler)
	}

	if handlers.ProviderProxyHandler != nil {
		mux.Handle("/provider/", handlers.ProviderProxyHandler)
	}
}

func routeGate(next http.Handler, settings repository.SystemSettingRepository, key string) http.Handler {
	if next == nil {
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !proxyRouteEnabled(settings, key) {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func proxyRouteEnabled(settings repository.SystemSettingRepository, key string) bool {
	if settings == nil {
		return proxyRouteEnabledByDefault(key)
	}

	value, err := settings.Get(key)
	if err != nil || value == "" {
		return proxyRouteEnabledByDefault(key)
	}

	return value != "false"
}

func proxyRouteEnabledByDefault(key string) bool {
	return true
}
