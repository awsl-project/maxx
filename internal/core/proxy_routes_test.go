package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterProxyRoutes_RegistersModelsRoute(t *testing.T) {
	mux := http.NewServeMux()
	called := false

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ModelsHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected /v1/models to be routed to ModelsHandler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
