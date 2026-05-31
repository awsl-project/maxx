package handler

import "net/http"

var protectedSelfServiceRoutePatterns = []string{
	"/api/providers",
	"/api/providers/",
	"/api/routes",
	"/api/routes/",
	"/api/projects",
	"/api/projects/",
	"/api/retry-configs",
	"/api/retry-configs/",
	"/api/provider-stats",
	"/api/provider-stats/",
	"/api/api-tokens",
	"/api/api-tokens/",
	"/api/model-mappings",
	"/api/model-mappings/",
	"/api/proxy-status",
	"/api/proxy-status/",
	"/api/model-prices",
	"/api/model-prices/",
	"/api/response-models",
	"/api/response-models/",
}

var publicSelfServiceRoutePatterns = []string{
	"/api/settings",
	"/api/settings/",
}

var selfServiceRoutePatterns = append(
	append([]string{}, protectedSelfServiceRoutePatterns...),
	publicSelfServiceRoutePatterns...,
)

// RegisterSelfServiceRoutes registers the admin and self-service HTTP endpoints under /api.
func RegisterSelfServiceRoutes(
	mux *http.ServeMux,
	wrap func(http.Handler) http.Handler,
	adminHandler http.Handler,
	selfServiceHandler http.Handler,
) {
	mux.Handle("/api/admin/", http.StripPrefix("/api", wrap(adminHandler)))

	wrappedSelfService := http.StripPrefix("/api", wrap(selfServiceHandler))
	for _, pattern := range protectedSelfServiceRoutePatterns {
		mux.Handle(pattern, wrappedSelfService)
	}

	publicSelfService := http.StripPrefix("/api", NoAuthMiddleware(selfServiceHandler))
	for _, pattern := range publicSelfServiceRoutePatterns {
		mux.Handle(pattern, publicSelfService)
	}
}
