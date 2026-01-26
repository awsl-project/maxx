package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/repository"
)

// ProjectProxyHandler wraps ProxyHandler to handle project-prefixed proxy requests
// like /{slug}/v1/messages, /{slug}/v1/chat/completions, etc.
type ProjectProxyHandler struct {
	proxyHandler *ProxyHandler
	projectRepo  repository.ProjectRepository
}

// NewProjectProxyHandler creates a new project proxy handler
func NewProjectProxyHandler(
	proxyHandler *ProxyHandler,
	projectRepo repository.ProjectRepository,
) *ProjectProxyHandler {
	return &ProjectProxyHandler{
		proxyHandler: proxyHandler,
		projectRepo:  projectRepo,
	}
}

// ServeHTTP handles project-prefixed proxy requests
func (h *ProjectProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse the path to extract project slug and API path
	// Expected format: /project/{slug}/v1/messages, /project/{slug}/v1/chat/completions, etc.
	slug, apiPath, ok := h.parseProjectPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "invalid project proxy path")
		return
	}

	// Look up project by slug
	project, err := h.projectRepo.GetBySlug(slug)
	if err != nil {
		log.Printf("[ProjectProxy] Project not found for slug: %s", slug)
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	log.Printf("[ProjectProxy] Routing request through project: %s (ID: %d)", project.Name, project.ID)

	// Set project ID header for the proxy handler to use
	r.Header.Set("X-Maxx-Project-ID", strings.TrimSpace(itoa(project.ID)))

	// Rewrite the URL path to the standard API path
	r.URL.Path = apiPath

	// Forward to the standard proxy handler
	h.proxyHandler.ServeHTTP(w, r)
}

// parseProjectPath extracts the project slug and API path from a project-prefixed URL
// Input: /project/my-project/v1/messages
// Output: ("my-project", "/v1/messages", true)
func (h *ProjectProxyHandler) parseProjectPath(path string) (slug, apiPath string, ok bool) {
	// Must start with /project/
	if !strings.HasPrefix(path, "/project/") {
		return "", "", false
	}

	// Remove /project/ prefix and split
	path = strings.TrimPrefix(path, "/project/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 {
		return "", "", false
	}

	slug = parts[0]
	apiPath = "/" + parts[1]

	// Validate this looks like a valid API path
	if !isValidAPIPath(apiPath) {
		return "", "", false
	}

	return slug, apiPath, true
}

// isValidAPIPath checks if the path is a known proxy API endpoint
func isValidAPIPath(path string) bool {
	// Claude API
	if strings.HasPrefix(path, "/v1/messages") {
		return true
	}
	// OpenAI API
	if strings.HasPrefix(path, "/v1/chat/completions") {
		return true
	}
	// Codex API
	if strings.HasPrefix(path, "/responses") {
		return true
	}
	// Gemini API
	if strings.HasPrefix(path, "/v1beta/models/") {
		return true
	}
	return false
}

// itoa converts uint64 to string without importing strconv
func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
