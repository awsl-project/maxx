package privacy_test

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestProjectPolicyIsolationAndUpdates(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewProjectRepository(db)
	project := &domain.Project{TenantID: 1, Name: "private", Slug: "private", BlockSecrets: true, EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeOpenAI}}
	if err := repo.Create(project); err != nil {
		t.Fatal(err)
	}
	cache := cached.NewProjectRepository(repo)
	if err := cache.Load(); err != nil {
		t.Fatal(err)
	}
	service := privacy.NewService(cache)
	body := []byte(`{"input":"sk-proj-testtesttesttesttesttesttesttest"}`)
	if rejection := service.Check(1, project.ID, body, "application/json"); rejection == nil || rejection.Code != "privacy_blocked" {
		t.Fatal("project credential was not blocked")
	}
	for _, tenant := range []uint64{2, 3} {
		if rejection := service.Check(tenant, project.ID, body, "application/json"); rejection == nil || rejection.Code != "privacy_check_failed" {
			t.Fatal("another tenant's policy accessible")
		}
		if _, err := service.SetProjectProtection(tenant, project.ID, false); err == nil {
			t.Fatal("another tenant's policy updated")
		}
	}
	if rejection := service.Check(1, project.ID+1, body, "application/json"); rejection == nil || rejection.HTTPStatusCode != 503 {
		t.Fatal("missing policy silently accepted")
	}
	if rejection := (*privacy.Service)(nil).Check(1, project.ID, body, "application/json"); rejection == nil {
		t.Fatal("missing service silently disabled protection")
	}
	if rejection := service.Check(1, 0, body, "application/json"); rejection != nil {
		t.Fatal("unassigned request unexpectedly protected")
	}
	if rejection := service.Check(1, project.ID, []byte(`{`), "application/json"); rejection == nil || rejection.Code != "privacy_check_failed" || rejection.HTTPStatusCode != 400 {
		t.Fatal("malformed body allowed")
	}
	before, err := cache.GetByID(1, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.SetProjectProtection(1, project.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !before.BlockSecrets {
		t.Fatal("cached read-only project mutated")
	}
	if updated.Name != project.Name || updated.Slug != project.Slug || len(updated.EnabledCustomRoutes) != 1 {
		t.Fatal("non-privacy settings changed")
	}
	if rejection := service.Check(1, project.ID, body, "application/json"); rejection != nil {
		t.Fatal("policy update not visible to request checking")
	}
}
