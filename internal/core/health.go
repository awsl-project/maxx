package core

import (
	"encoding/json"
	"net/http"

	"github.com/awsl-project/maxx/internal/version"
)

// HealthResponse defines the payload returned by /health.
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
}

// WriteHealthResponse writes a standard health check response.
func WriteHealthResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthResponse{
		Status:    "ok",
		Version:   version.Version,
		BuildTime: version.BuildTime,
	})
}
