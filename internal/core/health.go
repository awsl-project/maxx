package core

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

const healthCheckTimeout = 2 * time.Second

// HealthDependency describes a single external dependency required for healthy service operation.
type HealthDependency struct {
	Name  string
	Check func(context.Context) error
}

type healthResponse struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

// NewDatabaseHealthDependency returns a health check for the primary database connection.
func NewDatabaseHealthDependency(db *sqlite.DB) HealthDependency {
	return HealthDependency{
		Name: "database",
		Check: func(ctx context.Context) error {
			if db == nil {
				return errors.New("database is not initialized")
			}
			return db.PingContext(ctx)
		},
	}
}

// NewHealthHandler creates a shared /health handler that verifies all registered dependencies.
func NewHealthHandler(dependencies ...HealthDependency) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		resp := healthResponse{
			Status: "ok",
		}
		if len(dependencies) > 0 {
			resp.Dependencies = make(map[string]string, len(dependencies))
		}

		statusCode := http.StatusOK
		for _, dependency := range dependencies {
			if dependency.Name == "" || dependency.Check == nil {
				continue
			}

			if err := dependency.Check(ctx); err != nil {
				log.Printf("[Health] dependency %s check failed: %v", dependency.Name, err)
				resp.Status = "error"
				resp.Dependencies[dependency.Name] = "error"
				statusCode = http.StatusServiceUnavailable
				continue
			}

			resp.Dependencies[dependency.Name] = "ok"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("[Health] failed to encode response: %v", err)
		}
	}
}
