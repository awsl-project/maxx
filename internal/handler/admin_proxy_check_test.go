package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleProxyCheckRejectsEmptyURL(t *testing.T) {
	handler := NewAdminHandler(nil, nil, "")
	req := httptest.NewRequest(http.MethodPost, "/admin/proxy-check", strings.NewReader(`{"url":""}`))
	rec := httptest.NewRecorder()

	handler.handleProxyCheck(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleProxyCheckRequiresPost(t *testing.T) {
	handler := NewAdminHandler(nil, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/admin/proxy-check", nil)
	rec := httptest.NewRecorder()

	handler.handleProxyCheck(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
