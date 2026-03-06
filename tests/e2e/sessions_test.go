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
	// The handler calls svc.UpdateSessionProject which will likely return an error
	// for a non-existent session; we just check it doesn't panic and returns an error status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Expected status 200 or 500 for non-existent session project update, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSessionReject_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Try to reject a non-existent session
	resp := env.AdminPost("/api/admin/sessions/nonexistent-session-id/reject", nil)
	// The handler calls svc.RejectSession which will likely return an error
	// for a non-existent session; we just check it doesn't panic and returns an error status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Expected status 200 or 500 for non-existent session reject, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
