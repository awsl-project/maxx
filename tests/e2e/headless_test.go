package e2e_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/awsl-project/maxx/internal/handler"
)

// withStubStaticFS installs an in-memory web/dist so static serving is
// deterministic regardless of the working directory, restoring the previous
// value on cleanup. These tests must not run in parallel because handler.StaticFS
// is process-global.
func withStubStaticFS(t *testing.T, index string) {
	t.Helper()
	prev := handler.StaticFS
	handler.StaticFS = fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(index)},
	}
	t.Cleanup(func() { handler.StaticFS = prev })
}

// TestServeMode_UIServesStaticFiles verifies that in the default (UI) mode the
// server serves the embedded web UI at "/" and falls back to index.html for
// unknown client-side routes, while the API keeps working alongside it.
func TestServeMode_UIServesStaticFiles(t *testing.T) {
	const marker = "<!doctype html><title>maxx-test-ui</title>"
	withStubStaticFS(t, marker)
	env := newTestEnv(t, testEnvOptions{mountRoot: true, serveStatic: true})

	// Root serves index.html.
	resp := env.UnauthGet("/")
	AssertStatus(t, resp, http.StatusOK)
	if body := ReadBody(t, resp); !strings.Contains(body, "maxx-test-ui") {
		t.Fatalf("expected index.html served at /, got: %q", body)
	}

	// Unknown client-side route falls back to index.html (SPA routing).
	resp = env.UnauthGet("/dashboard/settings")
	AssertStatus(t, resp, http.StatusOK)
	if body := ReadBody(t, resp); !strings.Contains(body, "maxx-test-ui") {
		t.Fatalf("expected SPA fallback to index.html, got: %q", body)
	}

	// The API still works alongside static serving.
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// TestServeMode_HeadlessDoesNotServeUI verifies that headless mode (no UI) never
// serves static files — even when a static FS is available — while the API,
// health, and project-proxy routes remain fully functional.
func TestServeMode_HeadlessDoesNotServeUI(t *testing.T) {
	// A static FS is present but must NOT be served in headless mode.
	withStubStaticFS(t, "<title>should-not-be-served</title>")
	env := newTestEnv(t, testEnvOptions{mountRoot: true, serveStatic: false})

	// "/" is handled by the project proxy, not the static handler → 404, no UI.
	resp := env.UnauthGet("/")
	AssertStatus(t, resp, http.StatusNotFound)
	if body := ReadBody(t, resp); strings.Contains(body, "should-not-be-served") {
		t.Fatalf("headless mode leaked UI content: %q", body)
	}

	// A would-be client-side route is not served as the SPA shell either.
	resp = env.UnauthGet("/dashboard")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// Health and API endpoints remain available in headless mode.
	resp = env.UnauthGet("/health")
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
