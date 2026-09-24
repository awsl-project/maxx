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

func TestRedemptionCodeCreateWithAPITokenDebit_DebitsAndCreatesAtomically(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewRedemptionCodeRepository(db)
	tokenRepo := NewAPITokenRepository(db)
	token := &domain.APIToken{TenantID: 1, Token: "tok-self-create", TokenPrefix: "tok", Name: "user token", IsEnabled: true, QuotaBalance: 5_000_000_000}
	if err := tokenRepo.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}
	codes := []*domain.RedemptionCode{
		{CodeHash: domain.HashRedemptionCode("RC-SELF-1"), CodePrefix: domain.RedemptionCodePrefix("RC-SELF-1"), Amount: 1_000_000_000, CreatedByUserID: 7},
		{CodeHash: domain.HashRedemptionCode("RC-SELF-2"), CodePrefix: domain.RedemptionCodePrefix("RC-SELF-2"), Amount: 1_000_000_000, CreatedByUserID: 7},
	}

	if err := codeRepo.CreateWithAPITokenDebit(1, token.ID, 2_000_000_000, codes); err != nil {
		t.Fatalf("CreateWithAPITokenDebit: %v", err)
	}
	updated, err := tokenRepo.GetByID(1, token.ID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if updated.QuotaBalance != 3_000_000_000 {
		t.Fatalf("quota balance = %d, want 3 dollars", updated.QuotaBalance)
	}
	for _, code := range codes {
		if code.ID == 0 {
			t.Fatalf("code ID not populated: %+v", code)
		}
		stored, err := codeRepo.GetByID(1, code.ID)
		if err != nil {
			t.Fatalf("get code: %v", err)
		}
		if stored.CreatedByUserID != 7 || stored.Status != domain.RedemptionCodeStatusActive {
			t.Fatalf("stored code = %+v, want active user-owned code", stored)
		}
	}
}

func TestRedemptionCodeCreateWithAPITokenDebit_InsufficientBalanceCreatesNothing(t *testing.T) {
	db := newInviteTestDB(t)
	codeRepo := NewRedemptionCodeRepository(db)
	tokenRepo := NewAPITokenRepository(db)
	token := &domain.APIToken{TenantID: 1, Token: "tok-self-create-low", TokenPrefix: "tok", Name: "user token", IsEnabled: true, QuotaBalance: 1_000_000_000}
	if err := tokenRepo.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}
	codes := []*domain.RedemptionCode{
		{CodeHash: domain.HashRedemptionCode("RC-LOW-1"), CodePrefix: domain.RedemptionCodePrefix("RC-LOW-1"), Amount: 1_000_000_000, CreatedByUserID: 7},
		{CodeHash: domain.HashRedemptionCode("RC-LOW-2"), CodePrefix: domain.RedemptionCodePrefix("RC-LOW-2"), Amount: 1_000_000_000, CreatedByUserID: 7},
	}

	if err := codeRepo.CreateWithAPITokenDebit(1, token.ID, 2_000_000_000, codes); !errors.Is(err, domain.ErrAPITokenQuotaExhausted) {
		t.Fatalf("CreateWithAPITokenDebit error = %v, want quota exhausted", err)
	}
	updated, err := tokenRepo.GetByID(1, token.ID)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if updated.QuotaBalance != 1_000_000_000 {
		t.Fatalf("quota balance = %d, want unchanged", updated.QuotaBalance)
	}
	list, err := codeRepo.List(1)
	if err != nil {
		t.Fatalf("list codes: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("created codes = %d, want none", len(list))
	}
}
