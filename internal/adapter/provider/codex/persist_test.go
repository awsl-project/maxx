package codex

import (
	"sync"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestCloneForTokenPersistIsolatesMutations(t *testing.T) {
	orig := &domain.Provider{
		ID:   7,
		Name: "acct",
		Config: &domain.ProviderConfig{
			Codex: &domain.ProviderConfigCodex{
				AccessToken:  "old-at",
				RefreshToken: "old-rt",
				AccountID:    "acc-1",
			},
		},
	}

	cp, cpCfg := CloneForTokenPersist(orig)

	// Mutating the clone must not touch the original's shared Codex config.
	cpCfg.AccessToken = "new-at"
	cpCfg.RefreshToken = "new-rt"
	cpCfg.AccountID = "acc-2"

	if orig.Config.Codex.AccessToken != "old-at" || orig.Config.Codex.RefreshToken != "old-rt" || orig.Config.Codex.AccountID != "acc-1" {
		t.Fatalf("clone mutation leaked into original: %+v", orig.Config.Codex)
	}
	if cp.Config.Codex != cpCfg {
		t.Fatal("returned config pointer is not cp.Config.Codex")
	}
	if cp.Config == orig.Config {
		t.Fatal("clone shares the ProviderConfig pointer with the original")
	}
	if cp.ID != orig.ID || cp.Name != orig.Name {
		t.Fatal("clone lost scalar Provider fields")
	}
}

func TestCloneForTokenPersistNilSafe(t *testing.T) {
	cp, cpCfg := CloneForTokenPersist(nil)
	if cp == nil || cp.Config == nil || cp.Config.Codex == nil || cpCfg == nil {
		t.Fatal("nil input must yield a usable zero provider")
	}
	cpCfg.AccessToken = "x" // must not panic
}

// TestConcurrentPersistAndReadNoRace models the production race: one goroutine
// persists a rotated token (copy-on-write, swapping the shared pointer) while
// another reads token fields off the provider it currently holds. With
// copy-on-write neither goroutine mutates a struct the other reads, so `go test
// -race` stays clean. (Pre-fix, the persister mutated the shared struct in place
// while the reader read it — a data race.)
func TestConcurrentPersistAndReadNoRace(t *testing.T) {
	// Shared "cache slot" holding the current provider pointer, guarded like the
	// cached repository guards its map.
	var mu sync.RWMutex
	current := &domain.Provider{
		ID: 1,
		Config: &domain.ProviderConfig{
			Codex: &domain.ProviderConfigCodex{AccessToken: "at0", AccountID: "acc", RefreshToken: "rt0"},
		},
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Reader: repeatedly snapshots the current pointer and reads its fields,
	// exactly as Execute/getAccessToken read provider.Config.Codex.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			mu.RLock()
			p := current
			mu.RUnlock()
			cfg := p.Config.Codex
			_ = cfg.AccessToken
			_ = cfg.AccountID
			_ = cfg.RefreshToken
		}
	}()

	// Writer: copy-on-write persist, swapping the pointer under the lock.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			mu.RLock()
			base := current
			mu.RUnlock()
			cp, cpCfg := CloneForTokenPersist(base)
			cpCfg.AccessToken = "at-rotated"
			cpCfg.RefreshToken = "rt-rotated"
			mu.Lock()
			current = cp
			mu.Unlock()
		}
		close(stop)
	}()

	wg.Wait()
}
