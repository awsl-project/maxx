package service

import (
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

type stubSystemSettingRepo struct {
	values map[string]string
}

func (r *stubSystemSettingRepo) Get(key string) (string, error) {
	if r.values == nil {
		return "", nil
	}
	return r.values[key], nil
}

func (r *stubSystemSettingRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *stubSystemSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}

func (r *stubSystemSettingRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

func TestAdminServiceUpdateSettingRejectsInvalidPayloadOverrideRules(t *testing.T) {
	repo := &stubSystemSettingRepo{}
	svc := &AdminService{settingRepo: repo}

	err := svc.UpdateSetting(domain.SettingKeyPayloadOverrideRules, `[{"models":[{"name":"gpt-5.4","protocol":"codex"}],"params":{}}]`)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if len(repo.values) != 0 {
		t.Fatalf("expected invalid setting not to be persisted")
	}
}

func TestAdminServiceUpdateSettingRejectsInvalidProxyRequestsDisabledValue(t *testing.T) {
	repo := &stubSystemSettingRepo{}
	svc := &AdminService{settingRepo: repo}

	err := svc.UpdateSetting(domain.SettingKeyProxyRequestsDisabled, "maybe")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if len(repo.values) != 0 {
		t.Fatalf("expected invalid setting not to be persisted")
	}
}

func TestAdminServiceUpdateSettingAcceptsValidProxyRequestsDisabledValue(t *testing.T) {
	repo := &stubSystemSettingRepo{}
	svc := &AdminService{settingRepo: repo}

	err := svc.UpdateSetting(domain.SettingKeyProxyRequestsDisabled, "true")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got := repo.values[domain.SettingKeyProxyRequestsDisabled]; got != "true" {
		t.Fatalf("stored value = %q, want true", got)
	}
}

func TestAdminServiceUpdateSettingRejectsProxyRequestsDisabledValueWithWhitespace(t *testing.T) {
	repo := &stubSystemSettingRepo{}
	svc := &AdminService{settingRepo: repo}

	err := svc.UpdateSetting(domain.SettingKeyProxyRequestsDisabled, " true ")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if len(repo.values) != 0 {
		t.Fatalf("expected invalid setting not to be persisted")
	}
}

func TestAdminServiceUpdateSettingInvalidatesProxyBooleanCache(t *testing.T) {
	oldTTL := systemsettingcache.BooleanTTL
	systemsettingcache.BooleanTTL = time.Hour
	defer func() { systemsettingcache.BooleanTTL = oldTTL }()

	repo := &stubSystemSettingRepo{values: map[string]string{domain.SettingKeyProxyRequestsDisabled: "false"}}
	svc := &AdminService{settingRepo: repo}

	systemsettingcache.Invalidate(domain.SettingKeyProxyRequestsDisabled)
	if systemsettingcache.GetBoolean(repo, domain.SettingKeyProxyRequestsDisabled) {
		t.Fatal("expected initial cached value to be false")
	}

	if err := svc.UpdateSetting(domain.SettingKeyProxyRequestsDisabled, "true"); err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if !systemsettingcache.GetBoolean(repo, domain.SettingKeyProxyRequestsDisabled) {
		t.Fatal("expected cache invalidation to expose updated true value")
	}
}

func TestAdminServiceDeleteSettingInvalidatesProxyBooleanCache(t *testing.T) {
	oldTTL := systemsettingcache.BooleanTTL
	systemsettingcache.BooleanTTL = time.Hour
	defer func() { systemsettingcache.BooleanTTL = oldTTL }()

	repo := &stubSystemSettingRepo{values: map[string]string{domain.SettingKeyProxyRequestsDisabled: "true"}}
	svc := &AdminService{settingRepo: repo}

	systemsettingcache.Invalidate(domain.SettingKeyProxyRequestsDisabled)
	if !systemsettingcache.GetBoolean(repo, domain.SettingKeyProxyRequestsDisabled) {
		t.Fatal("expected initial cached value to be true")
	}

	if err := svc.DeleteSetting(domain.SettingKeyProxyRequestsDisabled); err != nil {
		t.Fatalf("expected delete to succeed, got %v", err)
	}
	if systemsettingcache.GetBoolean(repo, domain.SettingKeyProxyRequestsDisabled) {
		t.Fatal("expected cache invalidation to expose deleted false value")
	}
}

func TestBackupServiceImportSystemSettingsSkipsPayloadOverrideRules(t *testing.T) {
	repo := &stubSystemSettingRepo{
		values: map[string]string{},
	}

	svc := &BackupService{settingRepo: repo}
	result := domain.NewImportResult()
	svc.importSystemSettings(
		[]domain.BackupSystemSetting{
			{
				Key:   domain.SettingKeyPayloadOverrideRules,
				Value: `null`,
			},
			{Key: "other", Value: "new"},
		},
		domain.ImportOptions{ConflictStrategy: "skip"},
		result,
	)

	if !result.Success {
		t.Fatalf("expected import to succeed, got %+v", result)
	}
	if _, ok := repo.values[domain.SettingKeyPayloadOverrideRules]; ok {
		t.Fatalf("expected payload override rules to be ignored during import")
	}
	if got := repo.values["other"]; got != "new" {
		t.Fatalf("expected other setting to be imported, got %q", got)
	}
	summary, ok := result.Summary["systemSettings"]
	if !ok {
		t.Fatalf("expected systemSettings summary, got %+v", result.Summary)
	}
	if summary.Imported != 1 || summary.Skipped != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
