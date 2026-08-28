package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/awsl-project/maxx/internal/modelpriceupstream"
)

// handleModelPricesUpstreamPrices handles POST /admin/model-prices/upstream/prices.
func (h *AdminHandler) handleModelPricesUpstreamPrices(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeModelPriceUpstreamRequest(w, r)
	if !ok {
		return
	}
	result, err := h.svc.ListModelPricesFromExternalSource(r.Context(), req.Source)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, modelpriceupstream.ErrUnsupportedSource) {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type modelPriceUpstreamRequest struct {
	Source string `json:"source"`
}

func decodeModelPriceUpstreamRequest(w http.ResponseWriter, r *http.Request) (modelPriceUpstreamRequest, bool) {
	var req modelPriceUpstreamRequest
	if r.Body == nil || r.ContentLength == 0 {
		return req, true
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return req, true
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return req, false
	}
	return req, true
}
