package service

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestGlobalPrivacyBackupRemapsProvidersAndRestrictsTenant(t *testing.T) {
	sourceDB := newBackupServiceTestDB(t, "privacy-source.db")
	source := newBackupServiceForTest(t, sourceDB)
	provider := &domain.Provider{TenantID: 1, Name: "private", Type: "custom"}
	if err := source.providerRepo.Create(provider); err != nil {
		t.Fatal(err)
	}
	cfg := privacy.DefaultConfig()
	cfg.Enabled, cfg.Personal = true, true
	cfg.Words = []string{"Nightjar"}
	cfg.BypassProviders = []uint64{provider.ID}
	if _, err := privacy.NewService(source.settingRepo).SetConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	backup, err := source.Export(1)
	if err != nil {
		t.Fatal(err)
	}
	targetDB := newBackupServiceTestDB(t, "privacy-target.db")
	target := newBackupServiceForTest(t, targetDB)
	if err := target.providerRepo.Create(&domain.Provider{TenantID: 1, Name: "unrelated", Type: "custom"}); err != nil {
		t.Fatal(err)
	}
	result, err := target.Import(1, backup, domain.ImportOptions{ConflictStrategy: "overwrite"})
	if err != nil || !result.Success {
		t.Fatalf("import: %+v %v", result, err)
	}
	fresh, err := privacy.NewService(target.settingRepo).Configuration()
	if err != nil || !fresh.Enabled || len(fresh.Words) != 1 || len(fresh.BypassProviders) != 1 || fresh.BypassProviders[0] == provider.ID {
		t.Fatalf("global policy roundtrip: %+v %v", fresh, err)
	}
	resolved, err := target.providerRepo.GetByID(1, fresh.BypassProviders[0])
	if err != nil || resolved.Name != "private" {
		t.Fatal("exception resolved to unrelated provider")
	}

	other, err := source.Export(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range other.Data.SystemSettings {
		if setting.Key == privacy.SettingKey {
			t.Fatal("tenant backup exposed global privacy policy")
		}
	}
	before, _ := target.settingRepo.Get(privacy.SettingKey)
	if _, err := target.Import(2, backup, domain.ImportOptions{ConflictStrategy: "overwrite"}); err != nil {
		t.Fatal(err)
	}
	after, _ := target.settingRepo.Get(privacy.SettingKey)
	if before != after {
		t.Fatal("tenant backup changed global privacy policy")
	}

	admin := &AdminService{settingRepo: sqlite.NewSystemSettingRepository(targetDB)}
	settings, err := admin.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := settings[privacy.SettingKey]; ok {
		t.Fatal("generic settings exposed privacy configuration")
	}
}
