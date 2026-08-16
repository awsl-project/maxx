package multiinstance

import (
	"testing"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

type multiinstanceTestAdapter struct{}

func (a *multiinstanceTestAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

func (a *multiinstanceTestAdapter) Execute(*flow.Ctx, *domain.Provider) error {
	return nil
}

// A freshly-created project route on instance A must become matchable on
// instance B. This covers both metadata cache invalidation and provider adapter
// reconciliation, which are both required before Router.Match can return a
// usable upstream candidate.
func TestProjectRouteCreateInvalidatesPeerAndReconcilesAdapter(t *testing.T) {
	provideradapter.RegisterAdapterFactory("multiinstance-test", func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return &multiinstanceTestAdapter{}, nil
	})

	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	project := &domain.Project{
		TenantID:            domain.DefaultTenantID,
		Name:                "special-aws",
		Slug:                "aws",
		EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeClaude},
	}
	if err := a.Comp.Project.Create(project); err != nil {
		t.Fatalf("Create project on A: %v", err)
	}

	provider := &domain.Provider{
		TenantID:             domain.DefaultTenantID,
		Name:                 "shared-upstream",
		Type:                 "multiinstance-test",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
	if err := a.Comp.Provider.Create(provider); err != nil {
		t.Fatalf("Create provider on A: %v", err)
	}

	route := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProjectID:  project.ID,
		ProviderID: provider.ID,
		ClientType: domain.ClientTypeClaude,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := a.Comp.Route.Create(route); err != nil {
		t.Fatalf("Create route on A: %v", err)
	}

	if !waitFor(t, time.Second, func() bool {
		projectOnB, err := b.Comp.Project.GetBySlug(domain.DefaultTenantID, "aws")
		if err != nil || projectOnB == nil {
			return false
		}
		result, err := b.Router.Match(&router.MatchContext{
			TenantID:     domain.DefaultTenantID,
			ClientType:   domain.ClientTypeClaude,
			ProjectID:    projectOnB.ID,
			RequestModel: "claude-sonnet-5",
		})
		return err == nil &&
			result != nil &&
			len(result.Routes) == 1 &&
			result.Routes[0].Provider.ID == provider.ID &&
			result.Routes[0].ProviderAdapter != nil
	}) {
		t.Fatal("instance B never matched the project route created on A")
	}
}
