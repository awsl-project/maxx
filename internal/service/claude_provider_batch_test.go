package service

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestPreserveClaudeBatchOverwriteSettingsKeepsMaxConcurrency(t *testing.T) {
	candidate := &domain.Provider{MaxConcurrency: 0}
	existing := &domain.Provider{MaxConcurrency: 6}

	preserveClaudeBatchOverwriteSettings(candidate, existing)

	if candidate.MaxConcurrency != 6 {
		t.Fatalf("max concurrency = %d, want 6", candidate.MaxConcurrency)
	}
}
