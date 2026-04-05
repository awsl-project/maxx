package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelfServiceRoutePatterns_IncludeTrailingSlashVariants(t *testing.T) {
	mux := http.NewServeMux()
	RegisterSelfServiceRoutes(
		mux,
		func(h http.Handler) http.Handler { return h },
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	checked := make(map[string]bool, len(selfServiceRoutePatterns))
	for _, pattern := range selfServiceRoutePatterns {
		basePattern := strings.TrimSuffix(pattern, "/")
		if checked[basePattern] {
			continue
		}
		checked[basePattern] = true

		for _, candidate := range []string{basePattern, basePattern + "/"} {
			_, registeredPattern := mux.Handler(httptest.NewRequest(http.MethodGet, candidate, nil))
			if registeredPattern != candidate {
				t.Fatalf("expected self-service route %q to be registered, got %q", candidate, registeredPattern)
			}
		}
	}
}

func TestRegisterSelfServiceRoutes_RegistersProxyStatus(t *testing.T) {
	mux := http.NewServeMux()
	RegisterSelfServiceRoutes(
		mux,
		func(h http.Handler) http.Handler { return h },
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	for _, candidate := range []string{"/api/proxy-status", "/api/proxy-status/"} {
		_, registeredPattern := mux.Handler(httptest.NewRequest(http.MethodGet, candidate, nil))
		if registeredPattern != candidate {
			t.Fatalf("expected self-service route %q to be registered, got %q", candidate, registeredPattern)
		}
	}
}
