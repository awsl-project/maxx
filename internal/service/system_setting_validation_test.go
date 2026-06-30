package service

import (
	"errors"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
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

const validCodexReasoningGuardSetting = `{"enabled":true,"blocked_reasoning_tokens":[516,1034,1552],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`

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

func TestAdminServiceUpdateSettingValidatesCodexReasoningGuard(t *testing.T) {
	t.Run("accepts valid config", func(t *testing.T) {
		repo := &stubSystemSettingRepo{}
		svc := &AdminService{settingRepo: repo}

		if err := svc.UpdateSetting(domain.SettingKeyCodexReasoningGuard, validCodexReasoningGuardSetting); err != nil {
			t.Fatalf("expected valid setting, got %v", err)
		}
		if got := repo.values[domain.SettingKeyCodexReasoningGuard]; got != validCodexReasoningGuardSetting {
			t.Fatalf("expected setting to be persisted, got %q", got)
		}
	})

	t.Run("rejects invalid config", func(t *testing.T) {
		repo := &stubSystemSettingRepo{}
		svc := &AdminService{settingRepo: repo}

		err := svc.UpdateSetting(domain.SettingKeyCodexReasoningGuard, `{"enabled":true,"blocked_reasoning_tokens":[],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid input error, got %v", err)
		}
		if len(repo.values) != 0 {
			t.Fatalf("expected invalid setting not to be persisted")
		}
	})
}

func TestValidateSystemSettingValueCodexReasoningGuard(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:  "valid config",
			value: validCodexReasoningGuardSetting,
		},
		{
			name:    "invalid json",
			value:   `{"enabled":true`,
			wantErr: true,
		},
		{
			name:    "unknown field",
			value:   `{"enabled":true,"blocked_reasoning_tokens":[516],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream","unexpected":true}`,
			wantErr: true,
		},
		{
			name:    "empty blocked token",
			value:   `{"enabled":true,"blocked_reasoning_tokens":[],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`,
			wantErr: true,
		},
		{
			name:    "invalid max attempts",
			value:   `{"enabled":true,"blocked_reasoning_tokens":[516],"max_attempts":0,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`,
			wantErr: true,
		},
		{
			name:    "invalid status code",
			value:   `{"enabled":true,"blocked_reasoning_tokens":[516],"max_attempts":2,"status_code":200,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`,
			wantErr: true,
		},
		{
			name:    "empty mode",
			value:   `{"enabled":true,"blocked_reasoning_tokens":[516],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":" "}`,
			wantErr: true,
		},
		{
			name:    "non integer blocked token",
			value:   `{"enabled":true,"blocked_reasoning_tokens":["516"],"max_attempts":2,"status_code":502,"error_code":"reasoning_guard_triggered","mode":"non_stream"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSystemSettingValue(domain.SettingKeyCodexReasoningGuard, tt.value)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("expected invalid input error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
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
