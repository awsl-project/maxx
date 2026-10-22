package router

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestProjectPrivacyTenantIsolation(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewProjectRepository(db)
	project := &domain.Project{TenantID: 1, Name: "private", Slug: "private", BlockSecrets: true}
	if err := repo.Create(project); err != nil {
		t.Fatal(err)
	}
	cache := cached.NewProjectRepository(repo)
	if err := cache.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := (*Router)(nil).ProjectBlocksSecrets(1, project.ID); err == nil {
		t.Fatal("missing router silently disabled protection")
	}
	router := &Router{projectRepo: cache}
	if enabled, err := router.ProjectBlocksSecrets(1, project.ID); !enabled || err != nil {
		t.Fatalf("policy missing: %t %v", enabled, err)
	}
	// Check both cached and repository fallback paths.
	for _, tenant := range []uint64{2, 3} {
		if enabled, err := router.ProjectBlocksSecrets(tenant, project.ID); enabled || err == nil {
			t.Fatal("other tenant's project policy accessible")
		}
	}
	if enabled, err := router.ProjectBlocksSecrets(1, project.ID+1); enabled || err == nil {
		t.Fatal("missing policy silently accepted")
	}
	if enabled, err := router.ProjectBlocksSecrets(1, 0); enabled || err != nil {
		t.Fatal("unassigned request unexpectedly protected")
	}
}
