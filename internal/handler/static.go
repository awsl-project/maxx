package handler

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// StaticFS is the embedded filesystem for static files (set by main package)
var StaticFS fs.FS

// NewStaticHandler creates a handler for serving static files from web/dist
// If StaticFS is set, it uses the embedded filesystem; otherwise, reads from disk
func NewStaticHandler() http.Handler {
	if StaticFS != nil {
		return newEmbeddedStaticHandler(StaticFS)
	}
	return newFileSystemStaticHandler()
}

// newFileSystemStaticHandler serves static files from disk (web/dist)
func newFileSystemStaticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the web/dist directory path
		webDistPath := filepath.Join("web", "dist")

		// Clean the URL path
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "/" || urlPath == "." {
			urlPath = "/index.html"
		}
		urlPath = strings.TrimPrefix(urlPath, "/")

		// Build full file path
		filePath := filepath.Join(webDistPath, urlPath)

		// Try to open the file
		file, err := os.Open(filePath)
		if err != nil {
			// File not found, try index.html for SPA routing
			filePath = filepath.Join(webDistPath, "index.html")
			file, err = os.Open(filePath)
			if err != nil {
				// index.html also doesn't exist - frontend not built
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Frontend not built yet. Run 'task web-build' to build the frontend."))
				return
			}
		}
		defer file.Close()

		// Get file info for modification time
		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Serve the file
		http.ServeContent(w, r, filepath.Base(filePath), stat.ModTime(), file)
	})
}

// newEmbeddedStaticHandler serves static files from embedded filesystem
func newEmbeddedStaticHandler(fsys fs.FS) http.Handler {
	// Read index.html for SPA fallback
	indexContent, _ := fs.ReadFile(fsys, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the URL path
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "/" || urlPath == "." {
			urlPath = "index.html"
		} else {
			urlPath = strings.TrimPrefix(urlPath, "/")
		}

		// Try to read the file
		content, err := fs.ReadFile(fsys, urlPath)
		if err != nil {
			// File not found, serve index.html for SPA routing
			if indexContent != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write(indexContent)
				return
			}
			http.NotFound(w, r)
			return
		}

		// Set content type and serve
		w.Header().Set("Content-Type", getMimeType(urlPath))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}

func getMimeType(filePath string) string {
	ext := path.Ext(filePath)
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	default:
		return "application/octet-stream"
	}
}

// NewCombinedHandler creates a handler that routes project-prefixed proxy requests
// to the ProjectProxyHandler, and all other requests to the static file handler.
// This allows URLs like /my-project/v1/messages to be proxied through a specific project.
func NewCombinedHandler(projectProxyHandler *ProjectProxyHandler, staticHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this looks like a project-prefixed proxy request
		if isProjectProxyPath(r.URL.Path) {
			projectProxyHandler.ServeHTTP(w, r)
			return
		}

		// Otherwise, serve static files
		staticHandler.ServeHTTP(w, r)
	})
}

// isProjectProxyPath checks if the path looks like a project-prefixed proxy request
// e.g., /my-project/v1/messages, /my-project/v1/chat/completions, etc.
func isProjectProxyPath(urlPath string) bool {
	// Remove leading slash and split
	path := strings.TrimPrefix(urlPath, "/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 {
		return false
	}

	slug := parts[0]
	apiPath := "/" + parts[1]

	// Skip known non-project prefixes
	if slug == "admin" || slug == "antigravity" || slug == "v1" || slug == "v1beta" ||
		slug == "responses" || slug == "ws" || slug == "health" || slug == "assets" {
		return false
	}

	// Check if the API path looks like a known proxy endpoint
	return isValidAPIPath(apiPath)
}
