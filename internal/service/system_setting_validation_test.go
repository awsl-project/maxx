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

func TestValidateSystemSettingValueBooleanSettings(t *testing.T) {
	keys := []string{
		domain.SettingKeyForceRetryUpstreamErrors,
		domain.SettingKeyStreamTimeoutsEnabled,
		domain.SettingKeyRequestFailureDetailsEnabled,
		domain.SettingKeyProxyRequestsDisabled,
	}
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "true", value: "true"},
		{name: "false", value: "false"},
		{name: "trimmed uppercase", value: " TRUE "},
		{name: "empty", value: "", wantErr: true},
		{name: "invalid", value: "yes", wantErr: true},
	}

	for _, key := range keys {
		for _, tt := range tests {
			t.Run(key+"/"+tt.name, func(t *testing.T) {
				err := validateSystemSettingValue(key, tt.value)
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

func TestValidateStreamTimeoutMilliseconds(t *testing.T) {
	validKeys := []string{
		domain.SettingKeyStreamFirstEventTimeoutMS,
		domain.SettingKeyStreamIdleTimeoutMS,
	}
	for _, key := range validKeys {
		t.Run(key+" valid", func(t *testing.T) {
			if err := validateSystemSettingValue(key, "45000"); err != nil {
				t.Fatalf("validateSystemSettingValue() error = %v", err)
			}
		})
		t.Run(key+" too low", func(t *testing.T) {
			if err := validateSystemSettingValue(key, "999"); err == nil {
				t.Fatal("validateSystemSettingValue() error = nil, want error")
			}
		})
		t.Run(key+" too high", func(t *testing.T) {
			if err := validateSystemSettingValue(key, "600001"); err == nil {
				t.Fatal("validateSystemSettingValue() error = nil, want error")
			}
		})
	}
}
