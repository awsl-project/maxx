package executor

import (
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func providerQuotaEnabled(provider *domain.Provider) bool {
	return provider != nil && provider.Config != nil && provider.Config.QuotaEnabled
}

func (e *Executor) ensureAPITokenQuota(state *execState, proxyReq *domain.ProxyRequest, provider *domain.Provider) error {
	if !providerQuotaEnabled(provider) {
		return nil
	}
	if e.apiTokenRepo == nil || state.apiTokenID == 0 {
		return newAPITokenQuotaError()
	}
	token, err := e.apiTokenRepo.GetByID(state.tenantID, state.apiTokenID)
	if err != nil || token == nil || token.QuotaBalance == 0 {
		return newAPITokenQuotaError()
	}
	return nil
}

func (e *Executor) deductAPITokenQuota(state *execState, attempt *domain.ProxyUpstreamAttempt) {
	if e.apiTokenRepo == nil || state == nil || state.apiTokenID == 0 || attempt == nil || attempt.Cost == 0 {
		return
	}
	if err := e.apiTokenRepo.DeductQuotaBalanceToZero(state.tenantID, state.apiTokenID, attempt.Cost); err != nil {
		log.Printf("[Executor] API token quota deduct failed token=%d cost=%d: %v", state.apiTokenID, attempt.Cost, err)
	}
}

func rejectProxyRequestForQuota(proxyReq *domain.ProxyRequest, err error) {
	if proxyReq == nil {
		return
	}
	proxyReq.Status = "REJECTED"
	proxyReq.Error = err.Error()
	proxyReq.StatusCode = http.StatusForbidden
	proxyReq.EndTime = time.Now()
	proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
}

func newAPITokenQuotaError() *domain.ProxyError {
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrAPITokenQuotaExhausted, false, "当前令牌额度不足")
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.Code = "api_token_quota_exhausted"
	proxyErr.HTTPStatusCode = http.StatusForbidden
	return proxyErr
}
