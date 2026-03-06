package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestListProjects_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/projects")
	AssertStatus(t, resp, http.StatusOK)

	var projects []map[string]any
	DecodeJSON(t, resp, &projects)

	if len(projects) != 0 {
		t.Fatalf("Expected 0 projects, got %d", len(projects))
	}
}

func TestCreateProject(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "test-project",
		"slug":                "test-project",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["name"] != "test-project" {
		t.Fatalf("Expected name 'test-project', got %v", created["name"])
	}
	if created["slug"] != "test-project" {
		t.Fatalf("Expected slug 'test-project', got %v", created["slug"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/projects")
	AssertStatus(t, resp, http.StatusOK)

	var projects []map[string]any
	DecodeJSON(t, resp, &projects)

	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}
}

func TestGetProject_ByID(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "get-test-project",
		"slug":                "get-test-project",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/projects/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["name"] != "get-test-project" {
		t.Fatalf("Expected name 'get-test-project', got %v", fetched["name"])
	}
}

func TestUpdateProject(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "update-test-project",
		"slug":                "update-test-project",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the project
	updated := map[string]any{
		"name":                "updated-project-name",
		"slug":                "updated-project-slug",
		"enabledCustomRoutes": []string{"claude"},
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/projects/%d", int(id)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["name"] != "updated-project-name" {
		t.Fatalf("Expected name 'updated-project-name', got %v", result["name"])
	}
	if result["slug"] != "updated-project-slug" {
		t.Fatalf("Expected slug 'updated-project-slug', got %v", result["slug"])
	}
}

func TestDeleteProject(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "delete-test-project",
		"slug":                "delete-test-project",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the project
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/projects/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is gone from the list
	resp = env.AdminGet("/api/admin/projects")
	AssertStatus(t, resp, http.StatusOK)

	var remaining []map[string]any
	DecodeJSON(t, resp, &remaining)

	if len(remaining) != 0 {
		t.Fatalf("Expected 0 projects in list after delete, got %d", len(remaining))
	}
}

func TestGetProject_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/projects/999999")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestUpdateProject_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":                "ghost-project",
		"slug":                "ghost-project",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPut("/api/admin/projects/999999", body)
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestUpdateProject_MissingName(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project first
	project := map[string]any{
		"name":                "name-test-project",
		"slug":                "name-test-project",
		"enabledCustomRoutes": []string{},
	}
	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update with empty name
	body := map[string]any{
		"name":                "",
		"slug":                "name-test-project",
		"enabledCustomRoutes": []string{},
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/projects/%d", int(id)), body)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestUpdateProject_MissingSlug(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project first
	project := map[string]any{
		"name":                "slug-test-project",
		"slug":                "slug-test-project",
		"enabledCustomRoutes": []string{},
	}
	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update with empty slug
	body := map[string]any{
		"name":                "slug-test-project",
		"slug":                "",
		"enabledCustomRoutes": []string{},
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/projects/%d", int(id)), body)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateProject_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/projects", `{broken json`)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateProject_DuplicateSlug(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "dup-slug-project",
		"slug":                "dup-slug",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create another project with the same slug
	project2 := map[string]any{
		"name":                "dup-slug-project-2",
		"slug":                "dup-slug",
		"enabledCustomRoutes": []string{},
	}

	resp = env.AdminPost("/api/admin/projects", project2)
	// The service auto-deduplicates slug by appending a suffix
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	// The slug should be deduplicated to "dup-slug-2" per internal/repository/sqlite/project.go
	if created["slug"] != "dup-slug-2" {
		t.Fatalf("Expected deduplicated slug 'dup-slug-2', got %q", created["slug"])
	}
}

func TestGetProjectBySlug_Success(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":                "by-slug-project",
		"slug":                "by-slug-test",
		"enabledCustomRoutes": []string{},
	}

	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Get by slug
	resp = env.AdminGet("/api/admin/projects/by-slug/by-slug-test")
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["slug"] != "by-slug-test" {
		t.Fatalf("Expected slug 'by-slug-test', got %v", fetched["slug"])
	}
	if fetched["name"] != "by-slug-project" {
		t.Fatalf("Expected name 'by-slug-project', got %v", fetched["name"])
	}
}

func TestGetProjectBySlug_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/projects/by-slug/nonexistent-slug")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}
