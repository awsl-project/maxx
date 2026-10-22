package sqlite

import (
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestPrivacyScrubsPersistedCopiesIncludingDevMode(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const secret = "sk-proj-testtesttesttesttesttesttesttest"
	requestRepo := NewProxyRequestRepository(db)
	attemptRepo := NewProxyUpstreamAttemptRepository(db)
	for i, dev := range []bool{false, true} {
		req := buildTestProxyRequest("FAILED", i+1)
		req.DevMode = dev
		req.Error = "upstream rejected " + secret
		req.RequestInfo = &domain.RequestInfo{Body: `{"messages":[{"content":"` + secret + `"}],"password":"tiny"}`, Headers: map[string]string{"Authorization": "Bearer " + secret}}
		req.ResponseInfo = &domain.ResponseInfo{Body: `{"error":{"message":"` + secret + `"}}`, Headers: map[string]string{"Set-Cookie": "session=secret"}}
		if err := requestRepo.Create(req); err != nil {
			t.Fatal(err)
		}
		attempt := &domain.ProxyUpstreamAttempt{TenantID: req.TenantID, ProxyRequestID: req.ID, RequestInfo: req.RequestInfo, ResponseInfo: req.ResponseInfo, Error: req.Error, Status: "FAILED"}
		if err := attemptRepo.Create(attempt); err != nil {
			t.Fatal(err)
		}
		// Exercise updates too, not just insertion.
		if err := requestRepo.Update(req); err != nil {
			t.Fatal(err)
		}
		if err := attemptRepo.Update(attempt); err != nil {
			t.Fatal(err)
		}
		stored, err := requestRepo.GetByID(req.TenantID, req.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stored.RequestInfo.Body, secret) || strings.Contains(stored.RequestInfo.Body, "tiny") || strings.Contains(stored.ResponseInfo.Body, secret) || strings.Contains(stored.Error, secret) {
			t.Fatal("request archive contains credential")
		}
		var raw ProxyUpstreamAttempt
		if err := db.gorm.First(&raw, attempt.ID).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw.RequestInfo), secret) || strings.Contains(string(raw.ResponseInfo), secret) || strings.Contains(string(raw.Error), secret) {
			t.Fatal("attempt archive contains credential")
		}
		if !strings.Contains(req.RequestInfo.Body, secret) || !strings.Contains(req.ResponseInfo.Body, secret) || !strings.Contains(req.Error, secret) || !strings.Contains(attempt.ResponseInfo.Body, secret) {
			t.Fatal("live proxy objects mutated")
		}
	}
}
