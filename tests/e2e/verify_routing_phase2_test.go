package e2e_test

// Ad-hoc verification driver for the load-balancing + sticky change.
// Not part of the regression suite — meant to be invoked manually:
//   go test ./tests/e2e -run VerifyRoutingPhase2 -v
// The captured "verify> ..." log lines are the evidence.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
)

type hitMock struct {
	server *httptest.Server
	hits   atomic.Int64
}

func newHitMock(_ *testing.T, model string) *hitMock {
	hm := &hitMock{}
	hm.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hm.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-mock",
			"object":  "chat.completion",
			"model":   model,
			"created": 1700000000,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2,
			},
		})
	}))
	return hm
}

func TestVerifyRoutingPhase2(t *testing.T) {
	env := NewProxyTestEnv(t)

	// 3 mock upstreams with hit counters
	mocks := []*hitMock{
		newHitMock(t, "gpt-4o"),
		newHitMock(t, "gpt-4o"),
		newHitMock(t, "gpt-4o"),
	}
	defer func() {
		for _, m := range mocks {
			m.server.Close()
		}
	}()

	// 3 providers + routes; weights 7 / 2 / 1
	weights := []int{7, 2, 1}
	providerIDs := make([]uint64, 3)
	for i, m := range mocks {
		providerIDs[i] = createProvider(t, env, fmt.Sprintf("p%d", i+1), m.server.URL, []string{"openai"})
		resp := env.AdminPost("/api/admin/routes", map[string]any{
			"isEnabled":  true,
			"clientType": "openai",
			"providerID": providerIDs[i],
			"position":   i + 1,
			"weight":     weights[i],
		})
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("create route %d: status=%d body=%s", i, resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	// weighted_random strategy (no sticky yet)
	resp := env.AdminPost("/api/admin/routing-strategies", map[string]any{
		"projectID": 0,
		"type":      "weighted_random",
		"config":    nil,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create strategy: status=%d body=%s", resp.StatusCode, body)
	}
	var stratResp map[string]any
	DecodeJSON(t, resp, &stratResp)
	stratID := uint64(stratResp["id"].(float64))

	// Enable API token auth + create a token (sticky needs a non-zero APITokenID)
	resp = env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{
		"name": "verify-token", "description": "verify",
	})
	AssertStatus(t, resp, http.StatusCreated)
	var tokCreated map[string]any
	DecodeJSON(t, resp, &tokCreated)
	tokenStr := tokCreated["token"].(string)

	sendOne := func(sessionID string) int {
		resp := env.ProxyPost("/v1/chat/completions",
			openaiRequest("gpt-4o"),
			map[string]string{
				"Authorization": "Bearer " + tokenStr,
				"X-Session-Id":  sessionID,
			},
		)
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("proxy POST failed: %d", resp.StatusCode)
		}
		// Determine which mock got hit by sampling hit counts (won't tell us
		// which one if multiple changed; we check delta per-mock).
		return resp.StatusCode
	}

	hitsSnap := func() []int64 {
		out := make([]int64, len(mocks))
		for i, m := range mocks {
			out[i] = m.hits.Load()
		}
		return out
	}
	resetHits := func() {
		for _, m := range mocks {
			m.hits.Store(0)
		}
	}

	// =====================================================================
	// Phase A: distribution across many distinct sessions
	// Expected 70/20/10; with N=2000 each bucket's 99% CI is ~±3% (binomial
	// std dev ≈ sqrt(N*p*(1-p)) → roughly ±1% per σ; we allow ±3% ≈ 3σ).
	// =====================================================================
	resetHits()
	const N = 2000
	for i := 0; i < N; i++ {
		sendOne(fmt.Sprintf("phaseA-%d", i))
	}
	snapA := hitsSnap()
	total := snapA[0] + snapA[1] + snapA[2]
	pct := func(n int64) float64 { return float64(n) * 100 / float64(total) }
	t.Logf("verify> Phase A — distribution over %d distinct sessions: p1=%d (%.1f%%) p2=%d (%.1f%%) p3=%d (%.1f%%)",
		N, snapA[0], pct(snapA[0]), snapA[1], pct(snapA[1]), snapA[2], pct(snapA[2]))
	if total != int64(N) {
		t.Fatalf("phase A: expected %d total hits, got %d", N, total)
	}
	check := func(label string, got int64, wantPct float64, tolPct float64) {
		gotPct := pct(got)
		if gotPct < wantPct-tolPct || gotPct > wantPct+tolPct {
			t.Errorf("phase A %s: got %.1f%%, want ~%.1f%% (±%.1f%%)", label, gotPct, wantPct, tolPct)
		}
	}
	check("p1 (weight 7)", snapA[0], 70, 3)
	check("p2 (weight 2)", snapA[1], 20, 3)
	check("p3 (weight 1)", snapA[2], 10, 3)

	// =====================================================================
	// Phase B: seeded determinism — same session always lands on same provider
	// (even with sticky disabled, the seeded shuffle is deterministic).
	// =====================================================================
	resetHits()
	const repeats = 10
	for i := 0; i < repeats; i++ {
		sendOne("phaseB-stable")
	}
	snapB := hitsSnap()
	picked := -1
	picks := 0
	for i, h := range snapB {
		if h > 0 {
			if picked != -1 {
				picks = 2 // more than one provider got hit
			}
			picked = i
			picks++
		}
	}
	t.Logf("verify> Phase B — same session repeated %dx (sticky OFF): p1=%d p2=%d p3=%d → %s",
		repeats, snapB[0], snapB[1], snapB[2],
		map[bool]string{true: "DETERMINISTIC ✓", false: "SPLIT ✗"}[picks == 1])
	if picks != 1 || snapB[picked] != int64(repeats) {
		t.Errorf("phase B: expected all %d hits on one provider, got %v", repeats, snapB)
	}

	// =====================================================================
	// Phase C: turn sticky ON, send fresh session, verify sticky.Default()
	// records the binding AND subsequent calls keep hitting the same provider.
	// =====================================================================
	resp = env.AdminPut(fmt.Sprintf("/api/admin/routing-strategies/%d", stratID), map[string]any{
		"projectID": 0,
		"type":      "weighted_random",
		"config": map[string]any{
			"stickyEnabled":    true,
			"stickyScope":      "conversation",
			"stickyTTLSeconds": 600,
		},
	})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	// Let cached repo settle (route/strategy cache reload is sync, but be safe).
	time.Sleep(50 * time.Millisecond)

	resetHits()
	// Single warm-up hit creates the binding
	sendOne("phaseC-alpha")
	warmHits := hitsSnap()
	primedProvider := -1
	for i, h := range warmHits {
		if h > 0 {
			primedProvider = i
			break
		}
	}
	if primedProvider == -1 {
		t.Fatalf("phase C warm-up: no provider got hit")
	}
	t.Logf("verify> Phase C warm-up — session phaseC-alpha → provider p%d", primedProvider+1)

	const stickyRepeats = 30
	for i := 0; i < stickyRepeats; i++ {
		sendOne("phaseC-alpha")
	}
	snapC := hitsSnap()
	expectedPrimed := int64(1 + stickyRepeats) // warm-up + repeats
	totalC := snapC[0] + snapC[1] + snapC[2]
	t.Logf("verify> Phase C — sticky ON, session phaseC-alpha repeated %dx after warm-up: p1=%d p2=%d p3=%d (expect all %d on p%d)",
		stickyRepeats, snapC[0], snapC[1], snapC[2], expectedPrimed, primedProvider+1)
	if totalC != expectedPrimed {
		t.Errorf("phase C: hit total mismatch: got %d, want %d", totalC, expectedPrimed)
	}
	if snapC[primedProvider] != expectedPrimed {
		t.Errorf("phase C: sticky leak: provider p%d expected %d hits, got %d (others: %v)",
			primedProvider+1, expectedPrimed, snapC[primedProvider], snapC)
	}

	// =====================================================================
	// Phase D: cooldown the sticky-pinned provider; same session should fail
	// over to the others. The pinned provider must receive 0 new hits.
	// =====================================================================
	cooldown.Default().UpdateCooldown(providerIDs[primedProvider], "openai", "gpt-4o", time.Now().Add(30*time.Second))
	cooldown.Default().UpdateCooldown(providerIDs[primedProvider], "openai", "", time.Now().Add(30*time.Second))
	cooldown.Default().UpdateCooldown(providerIDs[primedProvider], "", "", time.Now().Add(30*time.Second))

	resetHits()
	const failoverRepeats = 20
	for i := 0; i < failoverRepeats; i++ {
		sendOne("phaseC-alpha") // same session — sticky still says primed, but cooldown filters it
	}
	snapD := hitsSnap()
	t.Logf("verify> Phase D — cooldown applied to p%d, same session phaseC-alpha repeated %dx: p1=%d p2=%d p3=%d",
		primedProvider+1, failoverRepeats, snapD[0], snapD[1], snapD[2])
	if snapD[primedProvider] != 0 {
		t.Errorf("phase D: cooled-down provider p%d still received %d hits", primedProvider+1, snapD[primedProvider])
	}
	totalD := snapD[0] + snapD[1] + snapD[2]
	if totalD != int64(failoverRepeats) {
		t.Errorf("phase D: total hits %d, want %d", totalD, failoverRepeats)
	}

	// Probe: under cooldown of p1, distinct sessions should still spread across
	// the remaining {p2, p3} by their weight ratio 2:1. (Phase D with one
	// session showed all 20 going to a single fallback — that's seeded
	// determinism for the same session, by design.)
	resetHits()
	const spreadN = 200
	for i := 0; i < spreadN; i++ {
		sendOne(fmt.Sprintf("probe-%d", i))
	}
	snapP := hitsSnap()
	t.Logf("verify> Probe — cooldown on p%d (weight %d), %d distinct sessions: p1=%d p2=%d p3=%d",
		primedProvider+1, weights[primedProvider], spreadN, snapP[0], snapP[1], snapP[2])
	if snapP[primedProvider] != 0 {
		t.Errorf("probe: cooled-down p%d received %d hits", primedProvider+1, snapP[primedProvider])
	}
	// With the primed provider cooled, traffic distributes between the
	// surviving providers by their weight ratio. Per-process random salt
	// means primedProvider varies across runs, so compute expected
	// counts dynamically and use a per-bucket 4σ tolerance (binomial
	// std dev sqrt(N·p·(1-p))). 4σ gives a Pr(flake) ≈ 6.3e-5 per
	// bucket per run — comfortably above the empirical noise floor
	// regardless of which weight bucket gets cooled.
	survivingWeight := 0
	for i, w := range weights {
		if i != primedProvider {
			survivingWeight += w
		}
	}
	const k = 4.0 // sigmas of tolerance
	for i, w := range weights {
		if i == primedProvider {
			continue
		}
		p := float64(w) / float64(survivingWeight)
		expected := float64(spreadN) * p
		sigma := math.Sqrt(float64(spreadN) * p * (1 - p))
		tol := k * sigma
		got := float64(snapP[i])
		if got < expected-tol || got > expected+tol {
			t.Errorf("probe: p%d (weight %d) got %d hits, want %.1f±%.1f (4σ) with p%d cooled",
				i+1, w, snapP[i], expected, tol, primedProvider+1)
		}
	}

	// Clean up cooldown state so other tests aren't affected
	cooldown.Default().ClearCooldown(providerIDs[primedProvider], "", "")
}

// togglableMock returns a mock upstream whose response status can be flipped
// at runtime via the returned setter. Used by TestVerifyRoutingErrorClass to
// drive 5xx/429 failure modes without restarting the server.
func togglableMock() (*httptest.Server, *hitMock, func(int)) {
	hm := &hitMock{}
	var status atomic.Int32
	status.Store(200)
	hm.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hm.hits.Add(1)
		code := int(status.Load())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code >= 400 {
			_, _ = w.Write([]byte(`{"error":{"type":"server_error","message":"forced"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-mock",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000000,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	return hm.server, hm, func(code int) { status.Store(int32(code)) }
}

// TestVerifyRoutingErrorClass exercises the failure / sticky-update story:
//
//   - When the sticky-pinned provider returns 5xx, dispatch fails over to the
//     next route. On success there, sticky overwrites to the new provider.
//   - Returning the previously-pinned provider to health does NOT cause
//     subsequent requests to drift back (sticky now points elsewhere).
//   - 429 follows the same path as 5xx — the implementation doesn't branch on
//     error class today; this just makes that explicit.
func TestVerifyRoutingErrorClass(t *testing.T) {
	env := NewProxyTestEnv(t)

	srvA, hmA, setA := togglableMock()
	defer srvA.Close()
	srvB, hmB, setB := togglableMock()
	defer srvB.Close()

	pA := createProvider(t, env, "pA", srvA.URL, []string{"openai"})
	pB := createProvider(t, env, "pB", srvB.URL, []string{"openai"})

	for _, pid := range []uint64{pA, pB} {
		resp := env.AdminPost("/api/admin/routes", map[string]any{
			"isEnabled":  true,
			"clientType": "openai",
			"providerID": pid,
			"position":   1,
			"weight":     1,
		})
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("create route: %d %s", resp.StatusCode, body)
		}
		resp.Body.Close()
	}

	resp := env.AdminPost("/api/admin/routing-strategies", map[string]any{
		"projectID": 0,
		"type":      "weighted_random",
		"config": map[string]any{
			"stickyEnabled":    true,
			"stickyScope":      "conversation",
			"stickyTTLSeconds": 600,
		},
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{"name": "err-tok"})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	token := created["token"].(string)

	hit := func(session string) int {
		r := env.ProxyPost("/v1/chat/completions",
			openaiRequest("gpt-4o"),
			map[string]string{"Authorization": "Bearer " + token, "X-Session-Id": session})
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, r.Body)
		return r.StatusCode
	}

	// Identify primed/spare by their roles so the test can drive either
	// initial sticky binding symmetrically. After warm-up sticky points
	// at one provider; we always fail *that* one and expect the dispatcher
	// to fall through to the other.
	type sideRefs struct {
		name string
		hits *hitMock
		set  func(int)
		id   uint64
	}
	a := sideRefs{name: "A", hits: hmA, set: setA, id: pA}
	b := sideRefs{name: "B", hits: hmB, set: setB, id: pB}

	// 1) Warm-up: both healthy, observe which one sticky pinned to.
	hmA.hits.Store(0)
	hmB.hits.Store(0)
	if code := hit("alpha"); code != 200 {
		t.Fatalf("warm-up status=%d", code)
	}
	var primed, spare sideRefs
	switch {
	case hmA.hits.Load() == 1 && hmB.hits.Load() == 0:
		primed, spare = a, b
	case hmB.hits.Load() == 1 && hmA.hits.Load() == 0:
		primed, spare = b, a
	default:
		t.Fatalf("warm-up: unexpected hits A=%d B=%d", hmA.hits.Load(), hmB.hits.Load())
	}
	t.Logf("verify> ErrorClass warm-up — sticky pinned to p%s (A=%d B=%d)",
		primed.name, hmA.hits.Load(), hmB.hits.Load())

	// 2) Flip the primed side to 500. Each subsequent request should attempt
	// the primed provider once (since sticky still says "go there"), get a
	// 500, fail over to the spare, succeed there, and write sticky=spare.
	// From the second request onward, sticky points at the spare and the
	// failing primed provider is bypassed entirely.
	primed.set(500)
	hmA.hits.Store(0)
	hmB.hits.Store(0)
	const reps = 10
	codes := make([]int, reps)
	for i := 0; i < reps; i++ {
		codes[i] = hit("alpha")
	}
	primedHits := primed.hits.hits.Load()
	spareHits := spare.hits.hits.Load()
	t.Logf("verify> ErrorClass primed=%s then %s→500 — %d requests for alpha: %s=%d %s=%d (codes=%v)",
		primed.name, primed.name, reps, primed.name, primedHits, spare.name, spareHits, codes)

	if spareHits == 0 {
		t.Errorf("ErrorClass: spare p%s never received traffic during p%s 500s; failover broken",
			spare.name, primed.name)
	}
	for i, c := range codes {
		if c != 200 {
			t.Errorf("ErrorClass: request %d returned %d, want 200 (failover should succeed)", i, c)
			break
		}
	}

	// 3) Heal the primed side. Subsequent requests should STILL hit the
	// spare (sticky was rebound to it after each success); no drift back.
	primed.set(200)
	hmA.hits.Store(0)
	hmB.hits.Store(0)
	for i := 0; i < reps; i++ {
		hit("alpha")
	}
	t.Logf("verify> ErrorClass p%s healed — %d requests for alpha: A=%d B=%d (expect all p%s, sticky points there now)",
		primed.name, reps, hmA.hits.Load(), hmB.hits.Load(), spare.name)
	if primed.hits.hits.Load() != 0 {
		t.Errorf("ErrorClass: sticky drifted back to recovered p%s; A=%d B=%d",
			primed.name, hmA.hits.Load(), hmB.hits.Load())
	}
	if spare.hits.hits.Load() != int64(reps) {
		t.Errorf("ErrorClass: expected %d hits on p%s, got %d", reps, spare.name, spare.hits.hits.Load())
	}

	// 4) Explicit 429 case. The dispatcher doesn't branch by error class
	// today — 4xx/429/5xx all fail over the same way — but Codex review
	// asked to make the 429 path observable so future rate-limit-specific
	// logic doesn't silently break the contract. Sticky now points at the
	// spare; flip it to 429 and verify the healed primed becomes the
	// failover target.
	cooldown.Default().ClearCooldown(primed.id, "", "")
	cooldown.Default().ClearCooldown(spare.id, "", "")
	spare.set(429)
	primed.set(200)
	hmA.hits.Store(0)
	hmB.hits.Store(0)
	for i := 0; i < reps; i++ {
		hit("alpha")
	}
	t.Logf("verify> ErrorClass 429 probe — p%s→429, %d requests for alpha: A=%d B=%d (expect majority p%s)",
		spare.name, reps, hmA.hits.Load(), hmB.hits.Load(), primed.name)
	if primed.hits.hits.Load() < int64(reps-1) {
		// One initial hit on the spare (the sticky-preferred) is expected;
		// everything after the failover should land on the primed.
		t.Errorf("ErrorClass 429: expected ≥%d hits on p%s after p%s→429, got A=%d B=%d",
			reps-1, primed.name, spare.name, hmA.hits.Load(), hmB.hits.Load())
	}

	// Cleanup: clear any cooldowns the dispatch loop may have applied
	// during the failure phase so other tests aren't affected.
	spare.set(200)
	cooldown.Default().ClearCooldown(pA, "", "")
	cooldown.Default().ClearCooldown(pB, "", "")
}
