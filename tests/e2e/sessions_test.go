package e2e_test

import (
	"net/http"
	"testing"
)

func TestListSessions_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/sessions")
	AssertStatus(t, resp, http.StatusOK)

	var sessions []map[string]any
	DecodeJSON(t, resp, &sessions)

	if len(sessions) != 0 {
		t.Fatalf("Expected 0 sessions, got %d", len(sessions))
	}
}

func TestSessionProject_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Try to set project on a non-existent session
	body := map[string]any{
		"projectID": 1,
	}
	resp := env.AdminPut("/api/admin/sessions/nonexistent-session-id/project", body)
	// Handler returns 500 for non-existent session (svc.UpdateSessionProject error).
	// This documents the current behavior; ideally it should be 404.
	AssertStatus(t, resp, http.StatusInternalServerError)
}

func TestSessionReject_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Try to reject a non-existent session
	resp := env.AdminPost("/api/admin/sessions/nonexistent-session-id/reject", nil)
	// Handler returns 500 for non-existent session (svc.RejectSession error).
	// This documents the current behavior; ideally it should be 404.
	AssertStatus(t, resp, http.StatusInternalServerError)
}
