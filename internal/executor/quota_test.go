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
