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

func TestValidateSystemSettingValueBooleanSettings(t *testing.T) {
	keys := []string{
		domain.SettingKeyForceRetryUpstreamErrors,
		domain.SettingKeyRequestFailureDetailsEnabled,
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
