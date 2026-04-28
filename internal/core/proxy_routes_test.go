package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterProxyRoutes_RegistersModelsRoute(t *testing.T) {
	mux := http.NewServeMux()
	calledPaths := make([]string, 0, 2)

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ModelsHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calledPaths = append(calledPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	for _, path := range []string{"/v1/models", "/v1beta/models"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNoContent)
		}
	}
	if len(calledPaths) != 2 {
		t.Fatalf("models handler calls = %v, want both model-list routes", calledPaths)
	}
}

func TestRegisterProxyRoutes_RoutesGeminiGenerationToProxy(t *testing.T) {
	mux := http.NewServeMux()
	proxyCalled := false

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
		ModelsHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("did not expect Gemini generation path to hit ModelsHandler")
		}),
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !proxyCalled {
		t.Fatal("expected Gemini generation path to be routed to ProxyHandler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
