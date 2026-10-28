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
		req.PrivacyEnabled = true
		req.Error = "upstream rejected " + secret
		req.RequestInfo = &domain.RequestInfo{URL: "https://alice:url-password@example.com/v1?api_key=url-secret&safe=hello", Body: `{"messages":[{"content":"` + secret + `"}],"password":"tiny"}`, Headers: map[string]string{"Authorization": "Bearer " + secret, "X-Service-Secret": "header-secret", "X-Debug": secret, "Content-Type": "application/json"}}
		req.ResponseInfo = &domain.ResponseInfo{Body: `{"error":{"message":"` + secret + `"}}`, Headers: map[string]string{"Set-Cookie": "session=secret", "X-Service-Secret": "response-header-secret"}}
		if err := requestRepo.Create(req); err != nil {
			t.Fatal(err)
		}
		attempt := &domain.ProxyUpstreamAttempt{PrivacyEnabled: true, TenantID: req.TenantID, ProxyRequestID: req.ID, RequestInfo: req.RequestInfo, ResponseInfo: req.ResponseInfo, Error: req.Error, Status: "FAILED"}
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
		var requestRaw ProxyRequest
		if err := db.gorm.First(&requestRaw, req.ID).Error; err != nil {
			t.Fatal(err)
		}
		for _, record := range []string{string(requestRaw.RequestInfo), string(requestRaw.ResponseInfo), string(raw.RequestInfo), string(raw.ResponseInfo)} {
			for _, credential := range []string{"url-password", "url-secret", "header-secret", "response-header-secret"} {
				if strings.Contains(record, credential) {
					t.Fatalf("snapshot contains credential %s", credential)
				}
			}
		}
		if stored.RequestInfo.Headers["Content-Type"] != "application/json" || !strings.Contains(stored.RequestInfo.URL, "safe=hello") {
			t.Fatal("safe snapshot metadata changed")
		}
		if req.RequestInfo.Headers["X-Service-Secret"] != "header-secret" || !strings.Contains(req.RequestInfo.URL, "url-secret") || req.ResponseInfo.Headers["X-Service-Secret"] != "response-header-secret" {
			t.Fatal("live metadata mutated")
		}
		if !strings.Contains(req.RequestInfo.Body, secret) || !strings.Contains(req.ResponseInfo.Body, secret) || !strings.Contains(req.Error, secret) || !strings.Contains(attempt.ResponseInfo.Body, secret) {
			t.Fatal("live proxy objects mutated")
		}
	}
}

func TestDisabledPrivacyPreservesArchiveBehavior(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requests := NewProxyRequestRepository(db)
	attempts := NewProxyUpstreamAttemptRepository(db)
	for _, devMode := range []bool{false, true} {
		req := buildTestProxyRequest("FAILED", 1)
		req.DevMode = devMode
		req.Error = "upstream password=Example1234"
		req.RequestInfo = &domain.RequestInfo{URL: "/v1/chat/completions?key=example", Body: "  {\"password\":\"tiny\"}\n", Headers: map[string]string{"Authorization": "Bearer example", "X-Service-Secret": "original"}}
		req.ResponseInfo = &domain.ResponseInfo{Body: "data: {\"password\":\"tiny\"}\n\ndata: [DONE]\n\n", Headers: map[string]string{"Set-Cookie": "example", "X-Service-Secret": "original"}}
		if err := requests.Create(req); err != nil {
			t.Fatal(err)
		}
		attempt := &domain.ProxyUpstreamAttempt{TenantID: req.TenantID, ProxyRequestID: req.ID, RequestInfo: req.RequestInfo, ResponseInfo: req.ResponseInfo, Error: req.Error}
		if err := attempts.Create(attempt); err != nil {
			t.Fatal(err)
		}
		if err := requests.Update(req); err != nil {
			t.Fatal(err)
		}
		if err := attempts.Update(attempt); err != nil {
			t.Fatal(err)
		}
		stored, err := requests.GetByID(req.TenantID, req.ID)
		if err != nil {
			t.Fatal(err)
		}
		storedAttempts, err := attempts.ListByProxyRequestID(req.ID)
		if err != nil || len(storedAttempts) != 1 {
			t.Fatalf("attempts: %v", err)
		}
		for _, record := range []struct {
			request   *domain.RequestInfo
			response  *domain.ResponseInfo
			errorText string
		}{{stored.RequestInfo, stored.ResponseInfo, stored.Error}, {storedAttempts[0].RequestInfo, storedAttempts[0].ResponseInfo, storedAttempts[0].Error}} {
			if record.request.Body != req.RequestInfo.Body || record.request.URL != req.RequestInfo.URL || record.response.Body != req.ResponseInfo.Body || record.errorText != req.Error {
				t.Fatal("disabled privacy rewrote archived content")
			}
			if record.request.Headers["X-Service-Secret"] != "original" || record.response.Headers["X-Service-Secret"] != "original" {
				t.Fatal("disabled privacy changed custom headers")
			}
			if record.request.Headers["Authorization"] == "Bearer example" || record.response.Headers["Set-Cookie"] == "example" {
				t.Fatal("existing sensitive-header protection lost")
			}
		}
	}
}
