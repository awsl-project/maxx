package router

import (
	_ "github.com/awsl-project/maxx/internal/adapter/provider/workbuddyai"
	"github.com/awsl-project/maxx/internal/domain"
	"testing"
)

func TestWorkBuddyStrictSupportModelsUsesMappedModelCandidates(t *testing.T) {
	r := newSupportModelRoutingTestRouter(t, true,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeOpenAI, IsEnabled: true, Position: 1},
			{ID: 2, TenantID: 1, ProviderID: 102, ClientType: domain.ClientTypeOpenAI, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 101, TenantID: 1, Type: "workbuddy-ai", Name: "pre-mapped-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}, Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "dummy"}}, SupportModels: []string{"gpt-5"}},
			{ID: 102, TenantID: 1, Type: "workbuddy-ai", Name: "mapped-target", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}, Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "dummy"}}, SupportModels: []string{"moonshotai/kimi-k3"}},
		},
	)

	result, err := r.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeOpenAI,
		RequestModel: "gpt-5",
		ModelCandidates: func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string {
			if requestModel == "gpt-5" {
				return []string{"moonshotai/kimi-k3"}
			}
			return []string{requestModel}
		},
	})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got := matchedProviderIDs(result); len(got) != 1 || got[0] != 102 {
		t.Fatalf("matched provider IDs = %v, want [102]", got)
	}
}

func TestWorkBuddySupportModelsPreservesCommonDefaultsAndConversion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client domain.ClientType
		strict bool
		models []string
	}{
		{"no configuration", domain.ClientTypeOpenAI, true, nil},
		{"strict off", domain.ClientTypeOpenAI, false, []string{"unrelated"}},
		{"converted route retains native-only strict policy", domain.ClientTypeClaude, true, []string{"unrelated"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &domain.Provider{ID: 103, TenantID: 1, Type: "workbuddy-ai", Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "dummy"}}, SupportModels: tc.models}
			r := newSupportModelRoutingTestRouter(t, tc.strict, []*domain.Route{{ID: 3, TenantID: 1, ProviderID: p.ID, ClientType: tc.client, IsEnabled: true}}, []*domain.Provider{p})
			result, err := r.Match(&MatchContext{TenantID: 1, ClientType: tc.client, RequestModel: "alias"})
			if err != nil || len(matchedProviderIDs(result)) != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}
