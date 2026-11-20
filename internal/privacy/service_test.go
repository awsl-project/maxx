package privacy_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"strings"
	"testing"
)

func TestGlobalPolicyPersistsAndCoversEveryTenant(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewSystemSettingRepository(db)
	svc := privacy.NewService(repo)
	const key = "sk-proj-testtesttesttesttesttesttesttest"
	body := privacy.TextRequest(key)
	if exchange, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json"); exchange != nil || rejection != nil {
		t.Fatal("global policy is not off by default")
	}
	cfg := privacy.DefaultConfig()
	cfg.Enabled = true
	if _, err := svc.SetConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []uint64{1, 2, 3} {
		exchange, rejection := svc.Prepare(context.Background(), tenant, body, body, "application/json")
		if rejection != nil || exchange == nil || strings.Contains(string(exchange.Body), key) {
			t.Fatalf("global policy skipped tenant %d: %v", tenant, rejection)
		}
		if string(exchange.RestoreState().RestoreJSON(exchange.Body)) != string(body) {
			t.Fatal("roundtrip failed")
		}
	}
	fresh, err := privacy.NewService(repo).Configuration()
	if err != nil || !fresh.Enabled {
		t.Fatal("policy did not persist")
	}
	cfg.Enabled = false
	if _, err := svc.SetConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	for _, contentType := range []string{"application/json", "application/octet-stream", "multipart/form-data; boundary=missing", "invalid;="} {
		if exchange, rejection := svc.Prepare(context.Background(), 1, []byte("invalid JSON "+key), nil, contentType); exchange != nil || rejection != nil {
			t.Fatal("off policy parsed a request")
		}
	}
}

type unavailablePrivacySettings struct{}

func (unavailablePrivacySettings) Get(string) (string, error) {
	return "", errors.New("storage unavailable")
}
func (unavailablePrivacySettings) Set(string, string) error { return errors.New("storage unavailable") }
func TestGlobalPolicyFailureDoesNotDisableProtection(t *testing.T) {
	for _, svc := range []*privacy.Service{privacy.NewService(unavailablePrivacySettings{}), privacy.NewService(nil)} {
		if exchange, rejection := svc.Prepare(context.Background(), 1, []byte(`{}`), nil, "application/json"); exchange == nil || rejection == nil || rejection.HTTPStatusCode != 503 {
			t.Fatal("storage failure disabled protection")
		}
	}
}
func TestGlobalPolicyBackupRemapsExceptions(t *testing.T) {
	cfg := privacy.DefaultConfig()
	cfg.Enabled, cfg.Personal = true, true
	cfg.Words = []string{"Nightjar"}
	cfg.BypassProviders = []uint64{7}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := privacy.ExportPolicy(raw, map[uint64]string{7: "private-upstream"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		names map[string]uint64
		want  uint64
	}{{map[string]uint64{"private-upstream": 19}, 19}, {map[string]uint64{}, 0}} {
		imported, err := privacy.ImportPolicy(exported, tc.names)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := privacy.ParseConfig(string(imported))
		if err != nil || !fresh.Enabled || !fresh.Personal || len(fresh.Words) != 1 {
			t.Fatal("backup lost global policy")
		}
		if tc.want == 0 && len(fresh.BypassProviders) != 0 || tc.want != 0 && (len(fresh.BypassProviders) != 1 || fresh.BypassProviders[0] != tc.want) {
			t.Fatal("backup reused a source database ID")
		}
	}
}
