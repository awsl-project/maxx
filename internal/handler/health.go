package handler

import (
	"encoding/json"
	"net/http"

	"github.com/awsl-project/maxx/internal/version"
)

// HealthHandler returns a public health-check endpoint that includes the
// running server version for operators and deployment probes.
func HealthHandler(service string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]string{
			"status":  "ok",
			"version": version.Version,
		}
		if service != "" {
			response["service"] = service
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}
