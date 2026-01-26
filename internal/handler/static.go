package handler

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// StaticFS is the embedded filesystem for static files (set by main package)
var StaticFS fs.FS

// staticFileCache caches file content and metadata for embedded files
type staticFileCache struct {
	content     []byte
	gzipped     []byte // pre-compressed content
	contentType string
	etag        string
	hasHash     bool // whether filename contains hash (can be cached long-term)
}

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
	// Cache for disk-based serving (with lazy loading)
	var cache sync.Map

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

		// Check cache first
		if cached, ok := cache.Load(urlPath); ok {
			serveFromCache(w, r, cached.(*staticFileCache))
			return
		}

		// Try to open the file
		file, err := os.Open(filePath)
		if err != nil {
			// File not found, try index.html for SPA routing
			filePath = filepath.Join(webDistPath, "index.html")
			urlPath = "index.html"

			if cached, ok := cache.Load(urlPath); ok {
				serveFromCache(w, r, cached.(*staticFileCache))
				return
			}

			file, err = os.Open(filePath)
			if err != nil {
				// index.html also doesn't exist - frontend not built
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Frontend not built yet. Run 'task web-build' to build the frontend."))
				return
			}
		}
		defer file.Close()

		// Read file content
		content, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Build cache entry
		cached := buildCacheEntry(urlPath, content)
		cache.Store(urlPath, cached)

		serveFromCache(w, r, cached)
	})
}

// newEmbeddedStaticHandler serves static files from embedded filesystem
func newEmbeddedStaticHandler(fsys fs.FS) http.Handler {
	// Pre-load all files into cache at startup
	cache := make(map[string]*staticFileCache)

	fs.WalkDir(fsys, ".", func(filePath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		content, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return nil
		}

		cache[filePath] = buildCacheEntry(filePath, content)
		return nil
	})

	// Get index.html for SPA fallback
	indexCache := cache["index.html"]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the URL path
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "/" || urlPath == "." {
			urlPath = "index.html"
		} else {
			urlPath = strings.TrimPrefix(urlPath, "/")
		}

		// Try to get from cache
		cached, ok := cache[urlPath]
		if !ok {
			// File not found, serve index.html for SPA routing
			if indexCache != nil {
				serveFromCache(w, r, indexCache)
				return
			}
			http.NotFound(w, r)
			return
		}

		serveFromCache(w, r, cached)
	})
}

// buildCacheEntry creates a cache entry with pre-computed metadata and gzip
func buildCacheEntry(urlPath string, content []byte) *staticFileCache {
	cached := &staticFileCache{
		content:     content,
		contentType: getMimeType(urlPath),
		etag:        fmt.Sprintf(`"%x"`, md5.Sum(content)),
		hasHash:     hasContentHash(urlPath),
	}

	// Pre-compress if it's a compressible type and large enough
	if isCompressible(cached.contentType) && len(content) > 1024 {
		var buf bytes.Buffer
		gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err == nil {
			gz.Write(content)
			gz.Close()
			// Only use gzip if it actually reduces size
			if buf.Len() < len(content) {
				cached.gzipped = buf.Bytes()
			}
		}
	}

	return cached
}

// serveFromCache serves a file from cache with proper headers
func serveFromCache(w http.ResponseWriter, r *http.Request, cached *staticFileCache) {
	// Set cache headers based on whether file has content hash
	if cached.hasHash {
		// Files with hash in name can be cached forever (immutable)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if cached.contentType == "text/html; charset=utf-8" {
		// HTML files should always be revalidated
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		// Other files without hash (favicon, logo, etc.) - cache for 1 day with revalidation
		w.Header().Set("Cache-Control", "public, max-age=86400, must-revalidate")
	}

	// Set ETag
	w.Header().Set("ETag", cached.etag)

	// Check If-None-Match for 304 response
	if r.Header.Get("If-None-Match") == cached.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Set content type
	w.Header().Set("Content-Type", cached.contentType)

	// Always set Vary header to ensure caches differentiate by Accept-Encoding
	w.Header().Set("Vary", "Accept-Encoding")

	// Check if client accepts gzip and we have gzipped content
	if cached.gzipped != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(cached.gzipped)))
		w.WriteHeader(http.StatusOK)
		w.Write(cached.gzipped)
		return
	}

	// Serve uncompressed
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(cached.content)))
	w.WriteHeader(http.StatusOK)
	w.Write(cached.content)
}

// hasContentHash checks if filename contains a content hash (Vite pattern: name-HASH.ext)
func hasContentHash(filePath string) bool {
	// Check if in assets directory (Vite puts hashed files here)
	if strings.HasPrefix(filePath, "assets/") {
		return true
	}

	// Check for hash pattern in filename: name-XXXXXXXX.ext
	base := path.Base(filePath)
	ext := path.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Look for pattern like "-CIq2CIyh" or "-6qBqSKe4" at the end
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		hash := name[idx+1:]
		// Vite hashes are typically 8 characters, alphanumeric
		if len(hash) >= 6 && len(hash) <= 12 {
			for _, c := range hash {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
					return false
				}
			}
			return true
		}
	}

	return false
}

// isCompressible checks if content type benefits from gzip compression
func isCompressible(contentType string) bool {
	compressible := []string{
		"text/html",
		"text/css",
		"text/plain",
		"text/xml",
		"application/javascript",
		"application/json",
		"application/xml",
		"image/svg+xml",
	}

	for _, ct := range compressible {
		if strings.HasPrefix(contentType, ct) {
			return true
		}
	}
	return false
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
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".pdf":
		return "application/pdf"
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
// e.g., /project/my-project/v1/messages, /project/my-project/v1/chat/completions, etc.
func isProjectProxyPath(urlPath string) bool {
	// Project routes must start with /project/
	return strings.HasPrefix(urlPath, "/project/")
}
