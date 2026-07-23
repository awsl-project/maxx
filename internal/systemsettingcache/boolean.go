package systemsettingcache

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/repository"
)

// BooleanTTL limits how often hot-path boolean settings re-read the repository.
// Keep it short so admin changes take effect quickly once the cache is invalidated.
var BooleanTTL = time.Second

type booleanCacheEntry struct {
	value     bool
	fetchedAt time.Time
}

var (
	booleanMu    sync.RWMutex
	booleanCache = make(map[string]booleanCacheEntry)
)

func GetBoolean(repo repository.SystemSettingRepository, key string) bool {
	if repo == nil {
		return false
	}

	now := time.Now()
	if value, ok := getFreshBoolean(key, now); ok {
		return value
	}

	rawValue, err := repo.Get(key)
	if err != nil {
		if value, ok := getCachedBoolean(key); ok {
			log.Printf("[SystemSettingCache] Failed to refresh %s, using cached value: %v", key, err)
			return value
		}
		log.Printf("[SystemSettingCache] Failed to read %s: %v", key, err)
		return false
	}

	value := normalizeBoolean(rawValue)
	storeBoolean(key, value, now)
	return value
}

func Invalidate(key string) {
	booleanMu.Lock()
	delete(booleanCache, key)
	booleanMu.Unlock()
}

func getFreshBoolean(key string, now time.Time) (bool, bool) {
	booleanMu.RLock()
	entry, ok := booleanCache[key]
	booleanMu.RUnlock()
	if !ok {
		return false, false
	}
	if BooleanTTL > 0 && now.Sub(entry.fetchedAt) <= BooleanTTL {
		return entry.value, true
	}
	return false, false
}

func getCachedBoolean(key string) (bool, bool) {
	booleanMu.RLock()
	entry, ok := booleanCache[key]
	booleanMu.RUnlock()
	if !ok {
		return false, false
	}
	return entry.value, true
}

func storeBoolean(key string, value bool, fetchedAt time.Time) {
	booleanMu.Lock()
	booleanCache[key] = booleanCacheEntry{value: value, fetchedAt: fetchedAt}
	booleanMu.Unlock()
}

func normalizeBoolean(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}
