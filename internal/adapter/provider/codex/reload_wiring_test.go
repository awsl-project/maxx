package codex

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// These mirror the duck-typed interfaces the router uses to inject callbacks
// (internal/router/router.go injectProviderUpdate). They use the literal func
// types so this test fails if a future signature change silently breaks the
// router's type assertions, leaving token persistence or reload-under-lock as
// dead code.
type providerUpdaterIface interface {
	SetProviderUpdateFunc(fn func(*domain.Provider) error)
}

type providerReloaderIface interface {
	SetProviderReloadFunc(fn func() (*domain.Provider, error))
}

func TestAdapterSatisfiesRouterWiringInterfaces(t *testing.T) {
	var a any = &CodexAdapter{}

	upd, ok := a.(providerUpdaterIface)
	if !ok {
		t.Fatal("*CodexAdapter does not satisfy providerUpdater (literal func sig) — token persistence wiring would be dead")
	}
	rel, ok := a.(providerReloaderIface)
	if !ok {
		t.Fatal("*CodexAdapter does not satisfy providerReloader (literal func sig) — reload-under-lock wiring would be dead")
	}

	reloaded := &domain.Provider{}
	upd.SetProviderUpdateFunc(func(*domain.Provider) error { return nil })
	rel.SetProviderReloadFunc(func() (*domain.Provider, error) { return reloaded, nil })

	ca := a.(*CodexAdapter)
	if ca.providerUpdate == nil {
		t.Fatal("providerUpdate not set after wiring")
	}
	if ca.providerReload == nil {
		t.Fatal("providerReload not set after wiring")
	}
	if got, _ := ca.providerReload(); got != reloaded {
		t.Fatal("providerReload returned unexpected value")
	}
}
