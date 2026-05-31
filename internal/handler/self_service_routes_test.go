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

func TestRegisterSelfServiceRoutes_SettingsBypassAuthWrapper(t *testing.T) {
	mux := http.NewServeMux()
	wrappedAuthCalled := false
	RegisterSelfServiceRoutes(
		mux,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wrappedAuthCalled = true
				http.Error(w, "auth required", http.StatusUnauthorized)
			})
		},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	settingsRec := httptest.NewRecorder()
	mux.ServeHTTP(settingsRec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if settingsRec.Code != http.StatusNoContent {
		t.Fatalf("expected public settings route to bypass auth wrapper, got status %d", settingsRec.Code)
	}
	if wrappedAuthCalled {
		t.Fatal("expected public settings route not to call auth wrapper")
	}

	protectedRec := httptest.NewRecorder()
	mux.ServeHTTP(protectedRec, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	if protectedRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected protected self-service route to use auth wrapper, got status %d", protectedRec.Code)
	}
	if !wrappedAuthCalled {
		t.Fatal("expected protected self-service route to call auth wrapper")
	}
}
