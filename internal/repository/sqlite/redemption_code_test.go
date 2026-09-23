package sqlite

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestRedemptionCodeRedeem_AddsBalanceOnceUnderConcurrency(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewRedemptionCodeRepository(db)
	tokenRepo := NewAPITokenRepository(db)
	plain := "RC-ONCE-123"
	code := &domain.RedemptionCode{
		TenantID:   1,
		CodeHash:   domain.HashRedemptionCode(plain),
		CodePrefix: domain.RedemptionCodePrefix(plain),
		Status:     domain.RedemptionCodeStatusActive,
		Amount:     25_000_000_000,
	}
	if err := codeRepo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}
	token := &domain.APIToken{TenantID: 1, Token: "tok-redemption", TokenPrefix: "tok", Name: "user token", IsEnabled: true}
	if err := tokenRepo.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	const attempts = 12
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := codeRepo.Redeem(1, domain.HashRedemptionCode(plain), 7, token.ID, time.Now())
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	used := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrRedemptionCodeUsed):
			used++
		default:
			t.Fatalf("unexpected redeem error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1", successes)
	}
	if used != attempts-1 {
		t.Fatalf("used errors = %d, want %d", used, attempts-1)
	}
	updated, err := tokenRepo.GetByID(1, token.ID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if updated.QuotaBalance != code.Amount {
		t.Fatalf("quota balance = %d, want %d", updated.QuotaBalance, code.Amount)
	}
}

func TestRedemptionCodeRedeem_Disabled(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewRedemptionCodeRepository(db)
	plain := "RC-DISABLED"
	code := &domain.RedemptionCode{
		TenantID:   1,
		CodeHash:   domain.HashRedemptionCode(plain),
		CodePrefix: domain.RedemptionCodePrefix(plain),
		Status:     domain.RedemptionCodeStatusDisabled,
		Amount:     1,
	}
	if err := codeRepo.Create(code); err != nil {
		t.Fatalf("create code: %v", err)
	}
	if _, err := codeRepo.Redeem(1, domain.HashRedemptionCode(plain), 7, 9, time.Now()); !errors.Is(err, domain.ErrRedemptionCodeDisabled) {
		t.Fatalf("redeem error = %v, want disabled", err)
	}
}
