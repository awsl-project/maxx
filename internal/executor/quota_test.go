package executor

import (
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type quotaTokenRepo struct {
	token         *domain.APIToken
	deductedToken uint64
	deductedCost  uint64
}

func (r *quotaTokenRepo) Create(token *domain.APIToken) error     { return nil }
func (r *quotaTokenRepo) Update(token *domain.APIToken) error     { return nil }
func (r *quotaTokenRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *quotaTokenRepo) DeleteExpired(tenantID uint64, now time.Time, inactiveExpiry time.Duration) ([]*domain.APIToken, error) {
	return nil, nil
}
func (r *quotaTokenRepo) GetByID(tenantID uint64, id uint64) (*domain.APIToken, error) {
	if r.token == nil || r.token.ID != id || r.token.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	return r.token, nil
}
func (r *quotaTokenRepo) GetByToken(tenantID uint64, token string) (*domain.APIToken, error) {
	return nil, errors.New("not implemented")
}
func (r *quotaTokenRepo) List(tenantID uint64) ([]*domain.APIToken, error) { return nil, nil }
func (r *quotaTokenRepo) UpdateLastSeen(tenantID uint64, id uint64, lastIP string, lastSeenAt time.Time) error {
	return nil
}
func (r *quotaTokenRepo) AddQuotaBalance(tenantID uint64, ids []uint64, amount uint64) (int64, error) {
	return int64(len(ids)), nil
}
func (r *quotaTokenRepo) DeductQuotaBalanceToZero(tenantID uint64, id uint64, amount uint64) error {
	r.deductedToken = id
	r.deductedCost = amount
	return nil
}

func TestEnsureAPITokenQuotaRejectsEnabledProviderWhenBalanceIsZero(t *testing.T) {
	repo := &quotaTokenRepo{token: &domain.APIToken{ID: 7, TenantID: 1, QuotaBalance: 0}}
	exec := &Executor{apiTokenRepo: repo}
	provider := &domain.Provider{Config: &domain.ProviderConfig{QuotaEnabled: true}}
	proxyReq := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, StartTime: time.Now()}

	err := exec.ensureAPITokenQuota(&execState{tenantID: 1, apiTokenID: 7}, proxyReq, provider)
	if err == nil {
		t.Fatal("expected quota error")
	}
	if !errors.Is(err, domain.ErrAPITokenQuotaExhausted) {
		t.Fatalf("error = %v, want ErrAPITokenQuotaExhausted", err)
	}
}

func TestEnsureAPITokenQuotaAllowsDisabledProviderWithZeroBalance(t *testing.T) {
	exec := &Executor{apiTokenRepo: &quotaTokenRepo{}}
	provider := &domain.Provider{Config: &domain.ProviderConfig{QuotaEnabled: false}}

	if err := exec.ensureAPITokenQuota(&execState{tenantID: 1, apiTokenID: 0}, &domain.ProxyRequest{}, provider); err != nil {
		t.Fatalf("ensureAPITokenQuota = %v, want nil", err)
	}
}

func TestDeductAPITokenQuotaUsesAttemptCost(t *testing.T) {
	repo := &quotaTokenRepo{}
	exec := &Executor{apiTokenRepo: repo}

	exec.deductAPITokenQuota(&execState{tenantID: 1, apiTokenID: 7}, &domain.ProxyUpstreamAttempt{Cost: 42})

	if repo.deductedToken != 7 || repo.deductedCost != 42 {
		t.Fatalf("deduct token/cost = %d/%d, want 7/42", repo.deductedToken, repo.deductedCost)
	}
}

type errorChargeSettingRepo struct {
	values map[string]string
}

func (r *errorChargeSettingRepo) Get(key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", domain.ErrNotFound
}
func (r *errorChargeSettingRepo) Set(key, value string) error              { return nil }
func (r *errorChargeSettingRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r *errorChargeSettingRepo) Delete(key string) error                  { return nil }

func TestUserPanelErrorChargeStartsAtThresholdAndContinuesUntilSuccess(t *testing.T) {
	repo := &quotaTokenRepo{token: &domain.APIToken{ID: 7, TenantID: 1, Description: "managed-by=maxx-user-panel;user-id=9", QuotaBalance: 50_000_000_000}}
	exec := &Executor{
		apiTokenRepo: repo,
		settingsRepo: &errorChargeSettingRepo{values: map[string]string{
			domain.SettingKeyUserPanelErrorChargeEnabled:   "true",
			domain.SettingKeyUserPanelErrorChargeCodes:     "429, 500",
			domain.SettingKeyUserPanelErrorChargeThreshold: "2",
			domain.SettingKeyUserPanelErrorChargeAmount:    "10",
		}},
		errorChargeStreak: make(map[errorChargeKey]int),
	}
	state := &execState{tenantID: 1, apiTokenID: 7}

	first := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "FAILED", StatusCode: 429}
	exec.applyUserPanelErrorCharge(state, first, nil)
	if repo.deductedCost != 0 || first.Cost != 0 {
		t.Fatalf("first error deducted/request cost = %d/%d, want 0/0", repo.deductedCost, first.Cost)
	}

	second := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "FAILED", StatusCode: 429}
	exec.applyUserPanelErrorCharge(state, second, nil)
	if repo.deductedCost != 10_000_000_000 || second.Cost != 10_000_000_000 {
		t.Fatalf("second error deducted/request cost = %d/%d, want 10 dollars", repo.deductedCost, second.Cost)
	}

	third := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "FAILED", StatusCode: 500}
	exec.applyUserPanelErrorCharge(state, third, nil)
	if repo.deductedCost != 10_000_000_000 || third.Cost != 10_000_000_000 {
		t.Fatalf("third consecutive configured error deducted/request cost = %d/%d, want 10 dollars", repo.deductedCost, third.Cost)
	}

	exec.applyUserPanelErrorCharge(state, &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "COMPLETED", StatusCode: 200}, nil)
	afterReset := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "FAILED", StatusCode: 429}
	repo.deductedCost = 0
	exec.applyUserPanelErrorCharge(state, afterReset, nil)
	if repo.deductedCost != 0 || afterReset.Cost != 0 {
		t.Fatalf("first error after success deducted/request cost = %d/%d, want 0/0", repo.deductedCost, afterReset.Cost)
	}
}

func TestUserPanelErrorChargeIgnoresRegularAPITokens(t *testing.T) {
	repo := &quotaTokenRepo{token: &domain.APIToken{ID: 7, TenantID: 1, Description: "regular", QuotaBalance: 50_000_000_000}}
	exec := &Executor{
		apiTokenRepo: repo,
		settingsRepo: &errorChargeSettingRepo{values: map[string]string{
			domain.SettingKeyUserPanelErrorChargeEnabled:   "true",
			domain.SettingKeyUserPanelErrorChargeCodes:     "429",
			domain.SettingKeyUserPanelErrorChargeThreshold: "1",
			domain.SettingKeyUserPanelErrorChargeAmount:    "10",
		}},
		errorChargeStreak: make(map[errorChargeKey]int),
	}
	req := &domain.ProxyRequest{TenantID: 1, APITokenID: 7, Status: "FAILED", StatusCode: 429}
	exec.applyUserPanelErrorCharge(&execState{tenantID: 1, apiTokenID: 7}, req, nil)
	if repo.deductedCost != 0 || req.Cost != 0 {
		t.Fatalf("regular token deducted/request cost = %d/%d, want 0/0", repo.deductedCost, req.Cost)
	}
}
