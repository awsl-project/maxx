package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

// handleProjectPrivacy is shared by the admin and tenant-scoped API routes.
func handleProjectPrivacy(w http.ResponseWriter, r *http.Request, svc *service.AdminService, id uint64) {
	if !CheckRBAC(r, "projects") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if id == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project id required"})
		return
	}
	var input struct {
		BlockSecrets *bool `json:"blockSecrets"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid privacy settings"})
		return
	}
	if input.BlockSecrets == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "blockSecrets is required"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid privacy settings"})
		return
	}
	project, err := svc.UpdateProjectPrivacy(maxxctx.GetTenantID(r.Context()), id, *input.BlockSecrets)
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err != nil {
		log.Printf("[Privacy] Update project policy failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, project)
}
