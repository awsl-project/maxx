package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

var errFakeRepo = errors.New("fake repo persistence error")

// recordingAttemptRepo captures Create/Update calls so the test can assert on
// the persisted attempt state. The real sqlite repo just writes to a DB; for
// unit-testing ExecuteOnce we only need the create/update lifecycle.
type recordingAttemptRepo struct {
	created []*domain.ProxyUpstreamAttempt
	updated []*domain.ProxyUpstreamAttempt
	nextID  uint64
}

func (r *recordingAttemptRepo) Create(a *domain.ProxyUpstreamAttempt) error {
	r.nextID++
	a.ID = r.nextID
	// Snapshot fields we care about so later Update mutations don't clobber
	// our view of what was persisted at creation time.
	snap := *a
	r.created = append(r.created, &snap)
	return nil
}

func (r *recordingAttemptRepo) Update(a *domain.ProxyUpstreamAttempt) error {
	snap := *a
	r.updated = append(r.updated, &snap)
	return nil
}

func (r *recordingAttemptRepo) ListByProxyRequestID(uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return nil, nil
}

func (r *recordingAttemptRepo) ListAll() ([]*domain.ProxyUpstreamAttempt, error) { return nil, nil }

func (r *recordingAttemptRepo) CountAll() (int64, error) { return 0, nil }

func (r *recordingAttemptRepo) StreamForCostCalc(int, func([]*domain.AttemptCostData) error) error {
	return nil
}

func (r *recordingAttemptRepo) BatchUpdateCosts(map[uint64]domain.AttemptCostUpdate) error {
	return nil
}

func (r *recordingAttemptRepo) MarkStaleAttemptsFailed() (int64, error) { return 0, nil }

func (r *recordingAttemptRepo) FixFailedAttemptsWithoutEndTime() (int64, error) { return 0, nil }

func (r *recordingAttemptRepo) ClearDetailOlderThan(time.Time, []string) (int64, error) {
	return 0, nil
}

// fakeMetricsAdapter emits a single EventMetrics event then returns. Models a
// provider adapter that successfully parsed usage from the upstream response.
type fakeMetricsAdapter struct {
	in, out uint64
	err     error
}

func (a *fakeMetricsAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *fakeMetricsAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	if ch := flow.GetEventChan(c); ch != nil {
		ch.SendMetrics(&domain.AdapterMetrics{InputTokens: a.in, OutputTokens: a.out})
	}
	return a.err
}

// TestExecuteOnce_PersistsAttemptAndMirrorsBilling pins the contract that
// drives the /provider/<id>/... direct-dispatch fix: a single adapter
// execution must produce a fully-billed attempt row and a ProxyRequest whose
// billing fields mirror the attempt. Before ExecuteOnce existed, the bypass
// path skipped attempt creation entirely — every request landed with
// proxyUpstreamAttemptCount=0 and cost=0.
func TestExecuteOnce_PersistsAttemptAndMirrorsBilling(t *testing.T) {
	repo := &recordingAttemptRepo{}
	e := &Executor{attemptRepo: repo}

	req := httptest.NewRequest(http.MethodPost, "/provider/1/v1/chat/completions", nil).
		WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{TenantID: 1, ID: 42}
	route := &domain.Route{ID: 99, ProjectID: 0, ProviderID: 7, ClientType: domain.ClientTypeOpenAI}
	provider := &domain.Provider{
		ID:   7,
		Type: "custom",
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				ClientMultiplier: map[domain.ClientType]uint64{domain.ClientTypeOpenAI: 64000},
			},
		},
	}
	adapter := &fakeMetricsAdapter{in: 7, out: 1290}

	attempt, err := e.ExecuteOnce(
		c, proxyReq, route, provider, adapter,
		domain.ClientTypeOpenAI, "gemini-2.5-flash-image", "gemini-2.5-flash-image",
		false, false,
	)
	if err != nil {
		t.Fatalf("ExecuteOnce returned error: %v", err)
	}
	if attempt == nil {
		t.Fatal("ExecuteOnce returned nil attempt")
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected exactly 1 attempt created, got %d", len(repo.created))
	}
	if len(repo.updated) == 0 {
		t.Fatal("expected at least one Update call to persist final state")
	}

	if got := repo.created[0].Status; got != "IN_PROGRESS" {
		t.Errorf("created attempt status = %q, want IN_PROGRESS", got)
	}

	if attempt.Status != "COMPLETED" {
		t.Errorf("final attempt status = %q, want COMPLETED", attempt.Status)
	}
	if attempt.InputTokenCount != 7 || attempt.OutputTokenCount != 1290 {
		t.Errorf("attempt tokens = in %d / out %d, want 7 / 1290",
			attempt.InputTokenCount, attempt.OutputTokenCount)
	}
	if attempt.Multiplier != 64000 {
		t.Errorf("attempt multiplier = %d, want 64000 (6.4× from clientMultiplier)",
			attempt.Multiplier)
	}

	if proxyReq.ProxyUpstreamAttemptCount != 1 {
		t.Errorf("ProxyUpstreamAttemptCount = %d, want 1",
			proxyReq.ProxyUpstreamAttemptCount)
	}
	if proxyReq.FinalProxyUpstreamAttemptID != attempt.ID {
		t.Errorf("FinalProxyUpstreamAttemptID = %d, want %d",
			proxyReq.FinalProxyUpstreamAttemptID, attempt.ID)
	}
	if proxyReq.InputTokenCount != 7 || proxyReq.OutputTokenCount != 1290 {
		t.Errorf("proxyReq tokens not mirrored: in %d / out %d, want 7 / 1290",
			proxyReq.InputTokenCount, proxyReq.OutputTokenCount)
	}
	if proxyReq.Multiplier != 64000 {
		t.Errorf("proxyReq multiplier not mirrored: %d, want 64000",
			proxyReq.Multiplier)
	}
}

// failingCreateAttemptRepo embeds recording behaviour but returns an error
// from Create — to verify the bypass path's fail-fast contract when the
// attempt row can't be persisted.
type failingCreateAttemptRepo struct {
	recordingAttemptRepo
	createErr error
}

func (r *failingCreateAttemptRepo) Create(a *domain.ProxyUpstreamAttempt) error {
	r.recordingAttemptRepo.created = append(r.recordingAttemptRepo.created, &domain.ProxyUpstreamAttempt{})
	return r.createErr
}

// TestExecuteOnce_FailsFastOnCreateError pins the safety contract added to
// address the desync concern: if the attempt row can't be persisted, do NOT
// continue running the adapter (which would mirror cost onto the request
// without any backing attempt row, leaving ProxyUpstreamAttemptCount=1 and
// FinalProxyUpstreamAttemptID=0 — exactly the corrupt state this PR fixes).
func TestExecuteOnce_FailsFastOnCreateError(t *testing.T) {
	repo := &failingCreateAttemptRepo{createErr: errFakeRepo}
	e := &Executor{attemptRepo: repo}

	req := httptest.NewRequest(http.MethodPost, "/provider/1/v1/chat/completions", nil).
		WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{TenantID: 1, ID: 42}
	route := &domain.Route{ID: 99, ProviderID: 7, ClientType: domain.ClientTypeOpenAI}
	provider := &domain.Provider{ID: 7, Type: "custom"}
	adapter := &fakeMetricsAdapter{in: 7, out: 1290} // would emit metrics if ever called

	attempt, err := e.ExecuteOnce(
		c, proxyReq, route, provider, adapter,
		domain.ClientTypeOpenAI, "m", "m", false, false,
	)
	if err != errFakeRepo {
		t.Fatalf("expected fail-fast Create error to be returned, got %v", err)
	}
	if attempt != nil {
		t.Errorf("expected nil attempt on Create failure, got %+v", attempt)
	}
	if proxyReq.ProxyUpstreamAttemptCount != 0 {
		t.Errorf("ProxyUpstreamAttemptCount = %d on Create failure; should stay 0 (no phantom counter)",
			proxyReq.ProxyUpstreamAttemptCount)
	}
	if proxyReq.Cost != 0 || proxyReq.Multiplier != 0 {
		t.Errorf("proxyReq billing fields touched on Create failure: cost=%d mult=%d",
			proxyReq.Cost, proxyReq.Multiplier)
	}
}

// failingUpdateAttemptRepo records a successful Create but fails the final
// Update, matching a late persistence failure after adapter.Execute has already
// produced an upstream response.
type failingUpdateAttemptRepo struct {
	recordingAttemptRepo
	updateErr error
}

func (r *failingUpdateAttemptRepo) Update(a *domain.ProxyUpstreamAttempt) error {
	snap := *a
	r.recordingAttemptRepo.updated = append(r.recordingAttemptRepo.updated, &snap)
	return r.updateErr
}

// TestExecuteOnce_FinalUpdateFailureStillMirrorsBilling documents the direct-
// dispatch contract for a late attempt update failure. At this point the
// adapter may already have written a successful response, so ExecuteOnce keeps
// the parent request billing mirror rather than converting the completed
// client response into an adapter error. The stale attempt row is left for the
// existing stale-attempt cleanup path.
func TestExecuteOnce_FinalUpdateFailureStillMirrorsBilling(t *testing.T) {
	repo := &failingUpdateAttemptRepo{updateErr: errFakeRepo}
	e := &Executor{attemptRepo: repo}

	req := httptest.NewRequest(http.MethodPost, "/provider/1/v1/chat/completions", nil).
		WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{TenantID: 1, ID: 42}
	route := &domain.Route{ID: 99, ProviderID: 7, ClientType: domain.ClientTypeOpenAI}
	provider := &domain.Provider{ID: 7, Type: "custom"}
	adapter := &fakeMetricsAdapter{in: 7, out: 1290}

	attempt, err := e.ExecuteOnce(
		c, proxyReq, route, provider, adapter,
		domain.ClientTypeOpenAI, "m", "m", false, false,
	)
	if err != nil {
		t.Fatalf("ExecuteOnce returned error on final Update failure: %v", err)
	}
	if attempt == nil || attempt.Status != "COMPLETED" {
		t.Fatalf("expected completed in-memory attempt, got %+v", attempt)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("expected exactly 1 final Update attempt, got %d", len(repo.updated))
	}
	if proxyReq.FinalProxyUpstreamAttemptID != attempt.ID {
		t.Errorf("FinalProxyUpstreamAttemptID = %d, want %d",
			proxyReq.FinalProxyUpstreamAttemptID, attempt.ID)
	}
	if proxyReq.InputTokenCount != 7 || proxyReq.OutputTokenCount != 1290 {
		t.Errorf("proxyReq tokens not mirrored after final Update failure: in %d / out %d, want 7 / 1290",
			proxyReq.InputTokenCount, proxyReq.OutputTokenCount)
	}
}

// TestExecuteOnce_RecordsFailedAttemptWithoutMirroringCost guards the failure
// path: when the adapter errors, the attempt row still gets persisted with
// Status=FAILED and the multiplier is preserved (for audit), but the request's
// billing fields stay zero — we don't bill failed upstream calls.
func TestExecuteOnce_RecordsFailedAttemptWithoutMirroringCost(t *testing.T) {
	repo := &recordingAttemptRepo{}
	e := &Executor{attemptRepo: repo}

	req := httptest.NewRequest(http.MethodPost, "/provider/1/v1/chat/completions", nil).
		WithContext(context.Background())
	c := flow.NewCtx(httptest.NewRecorder(), req)

	proxyReq := &domain.ProxyRequest{TenantID: 1, ID: 42}
	route := &domain.Route{ID: 99, ProviderID: 7, ClientType: domain.ClientTypeOpenAI}
	provider := &domain.Provider{ID: 7, Type: "custom"}
	adapter := &fakeMetricsAdapter{err: &domain.ProxyError{Message: "boom"}}

	attempt, err := e.ExecuteOnce(
		c, proxyReq, route, provider, adapter,
		domain.ClientTypeOpenAI, "m", "m", false, false,
	)
	if err == nil {
		t.Fatal("expected error from failing adapter")
	}
	if attempt == nil || attempt.Status != "FAILED" {
		t.Fatalf("expected attempt.Status=FAILED, got %+v", attempt)
	}
	if proxyReq.FinalProxyUpstreamAttemptID != 0 {
		t.Errorf("FinalProxyUpstreamAttemptID = %d on failure path; should stay zero",
			proxyReq.FinalProxyUpstreamAttemptID)
	}
	if proxyReq.Cost != 0 {
		t.Errorf("proxyReq.Cost = %d on failure path; should stay zero", proxyReq.Cost)
	}
}
