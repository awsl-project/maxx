package service

import (
	"github.com/awsl-project/maxx/internal/domain"
	"testing"
)

func TestWorkBuddyCredentialReplacement(t *testing.T) {
	old := &domain.Provider{Type: "workbuddy-ai", Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "old-dummy", RefreshToken: "old-refresh"}}}
	for _, access := range []string{"", "new-dummy"} {
		in := &domain.Provider{Type: "workbuddy-ai", Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: access}}}
		preserveEmptyProviderSecrets(old, in)
		if access == "" && in.Config.WorkBuddyAI.RefreshToken != "old-refresh" {
			t.Fatal("unchanged credentials lost")
		}
		if access != "" && in.Config.WorkBuddyAI.RefreshToken != "" {
			t.Fatal("replacement inherited old refresh credential")
		}
	}
}
