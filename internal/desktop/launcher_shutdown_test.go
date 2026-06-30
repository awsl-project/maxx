package desktop

import (
	"context"
	"testing"

	"github.com/awsl-project/maxx/internal/core"
)

func TestShutdownResourcesIsIdempotent(t *testing.T) {
	cleanupCalls := 0
	app := &LauncherApp{
		components: &core.ServerComponents{
			CoordinatorCleanup: func() { cleanupCalls++ },
		},
	}

	app.shutdownResources(context.Background())
	app.shutdownResources(context.Background())

	if cleanupCalls != 1 {
		t.Fatalf("CoordinatorCleanup calls = %d, want 1", cleanupCalls)
	}
	if app.components != nil {
		t.Fatalf("components = %#v, want nil after shutdown", app.components)
	}
}

func TestTrayQuitFuncCanBeRegisteredAndInvoked(t *testing.T) {
	app := &LauncherApp{}
	calls := 0
	app.SetTrayQuitFunc(func() { calls++ })

	app.quitTray()

	if calls != 1 {
		t.Fatalf("tray quit calls = %d, want 1", calls)
	}
}
