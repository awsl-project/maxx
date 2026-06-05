package handler

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeFromCache_GzipPathSetsContentEncoding ensures that when a gzip-capable
// client requests a file for which we have pre-compressed bytes, the response
// carries Content-Encoding: gzip and the body is the compressed bytes (not the
// raw content). This is a regression guard: a previous proposed fix accidentally
// dropped the Content-Encoding header while changing the Vary strategy.
func TestServeFromCache_GzipPathSetsContentEncoding(t *testing.T) {
	// Build a cache entry whose gzipped field is non-nil.
	// buildCacheEntry only pre-compresses content > 1024 bytes of a
	// compressible MIME type, so use a large JS payload.
	content := []byte(strings.Repeat("function hello(){return 42;}\n", 50))
	cached := buildCacheEntry("assets/app-abc123.js", content)
	if cached.gzipped == nil {
		t.Skip("pre-compression did not produce a gzipped entry; check buildCacheEntry thresholds")
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()
	serveFromCache(rec, req, cached)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want \"gzip\"", got)
	}

	// The body must be valid gzip that decompresses to the original content.
	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("response body is not valid gzip: %v", err)
	}
	defer gr.Close()
	got, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("reading decompressed body: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("decompressed body mismatch: got %d bytes, want %d bytes", len(got), len(content))
	}
}

// TestServeFromCache_VaryPreservesExistingValues verifies that serveFromCache
// uses Add (not Set) for the Vary header so that values written by upstream
// middleware (such as the CORS middleware's "Vary: Origin") are not clobbered.
func TestServeFromCache_VaryPreservesExistingValues(t *testing.T) {
	content := []byte("<html><body>hi</body></html>")
	cached := buildCacheEntry("index.html", content)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	// Simulate CORS middleware having already written Vary: Origin before the
	// static handler runs.
	rec.Header().Add("Vary", "Origin")

	serveFromCache(rec, req, cached)

	vary := rec.Header().Values("Vary")
	hasOrigin := false
	hasEncoding := false
	for _, v := range vary {
		if v == "Origin" {
			hasOrigin = true
		}
		if v == "Accept-Encoding" {
			hasEncoding = true
		}
	}
	if !hasOrigin {
		t.Fatalf("Vary header lost upstream \"Origin\" value; got %v", vary)
	}
	if !hasEncoding {
		t.Fatalf("Vary header missing \"Accept-Encoding\"; got %v", vary)
	}
}

// TestServeFromCache_NonGzipClientGetsUncompressed verifies that a client that
// does not advertise gzip support receives the uncompressed content without a
// Content-Encoding header, even when a pre-compressed version is available.
func TestServeFromCache_NonGzipClientGetsUncompressed(t *testing.T) {
	content := []byte(strings.Repeat("function hello(){return 42;}\n", 50))
	cached := buildCacheEntry("assets/app-abc123.js", content)
	if cached.gzipped == nil {
		t.Skip("pre-compression not available; check buildCacheEntry thresholds")
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	// No Accept-Encoding: gzip
	rec := httptest.NewRecorder()
	serveFromCache(rec, req, cached)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for non-gzip client", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(content) {
		t.Fatalf("body mismatch: got %d bytes, want %d bytes", len(body), len(content))
	}
}
