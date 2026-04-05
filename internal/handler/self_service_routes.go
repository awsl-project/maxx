package handler

import "net/http"

var selfServiceRoutePatterns = []string{
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
	"/api/settings",
	"/api/settings/",
	"/api/proxy-status",
	"/api/proxy-status/",
	"/api/model-prices",
	"/api/model-prices/",
	"/api/response-models",
	"/api/response-models/",
}

// RegisterSelfServiceRoutes registers the admin and self-service HTTP endpoints under /api.
func RegisterSelfServiceRoutes(
	mux *http.ServeMux,
	wrap func(http.Handler) http.Handler,
	adminHandler http.Handler,
	selfServiceHandler http.Handler,
) {
	mux.Handle("/api/admin/", http.StripPrefix("/api", wrap(adminHandler)))

	wrappedSelfService := http.StripPrefix("/api", wrap(selfServiceHandler))
	for _, pattern := range selfServiceRoutePatterns {
		mux.Handle(pattern, wrappedSelfService)
	}
}
