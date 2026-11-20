package pii

import (
	"os"
	"path/filepath"
)

// Model files contain public weights; original request texts are never cached
// on disk. Follow Maxx's cache override and the deployment's standard cache dir.
func cacheRoot() string {
	if dir := os.Getenv("MAXX_PRIVACY_CACHE_DIR"); dir != "" {
		return dir
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "maxx", "privacy")
}
