package bedrock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
)

func TestExtractShortName(t *testing.T) {
	cases := []struct {
		id    string
		want  string
		match bool
	}{
		{"us.anthropic.claude-opus-4-7-20260115-v1:0", "claude-opus-4-7", true},
		{"anthropic.claude-3-5-sonnet-20241022-v2:0", "claude-3-5-sonnet", true},
		{"eu.anthropic.claude-sonnet-4-5-20250514-v1:0", "claude-sonnet-4-5", true},
		{"apac.anthropic.claude-haiku-4-5-20251001-v1:0", "claude-haiku-4-5", true},
		{"anthropic.claude-opus-4-20250514-v1:0", "claude-opus-4", true},
		// Non-Anthropic / malformed should not match.
		{"amazon.titan-text-lite-v1", "", false},
		{"us.meta.llama3-70b-instruct-v1:0", "", false},
		// Legacy "claude-v2" has no release date, but foundation-model
		// matching now accepts it — the "v2" digit satisfies the short-
		// name requirement and is preserved as part of the name itself.
		{"anthropic.claude-v2", "claude-v2", true},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := extractShortName(c.id)
		if ok != c.match || got != c.want {
			t.Errorf("extractShortName(%q) = (%q,%v); want (%q,%v)", c.id, got, ok, c.want, c.match)
		}
	}
}

func TestProfileDiscovererLookupAndPagination(t *testing.T) {
	var profileHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			atomic.AddInt32(&profileHits, 1)
			switch r.URL.Query().Get("nextToken") {
			case "":
				fmt.Fprint(w, `{
					"inferenceProfileSummaries":[
						{"inferenceProfileId":"us.anthropic.claude-opus-4-7-20260115-v1:0","status":"ACTIVE"},
						{"inferenceProfileId":"us.anthropic.claude-sonnet-4-5-20250514-v1:0","status":"ACTIVE"},
						{"inferenceProfileId":"us.meta.llama3-70b-instruct-v1:0","status":"ACTIVE"}
					],
					"nextToken":"page2"
				}`)
			case "page2":
				fmt.Fprint(w, `{
					"inferenceProfileSummaries":[
						{"inferenceProfileId":"us.anthropic.claude-opus-4-7-20251001-v1:0","status":"ACTIVE"}
					]
				}`)
			default:
				http.Error(w, "unexpected token", http.StatusBadRequest)
			}
		case "/foundation-models":
			fmt.Fprint(w, `{"modelSummaries":[]}`)
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	id, ok := d.Lookup(context.Background(), "claude-opus-4-7")
	if !ok {
		t.Fatalf("expected claude-opus-4-7 to resolve")
	}
	// Newer date (20260115 > 20251001) should win across pages.
	if !strings.Contains(id, "20260115") {
		t.Errorf("expected newest profile, got %q", id)
	}

	if _, ok := d.Lookup(context.Background(), "claude-sonnet-4-5"); !ok {
		t.Error("expected claude-sonnet-4-5 to resolve")
	}
	if _, ok := d.Lookup(context.Background(), "claude-nonexistent"); ok {
		t.Error("unexpected hit for unknown model")
	}

	// Cache should prevent a second round-trip.
	before := atomic.LoadInt32(&profileHits)
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&profileHits)-before > 0 {
		t.Error("cached Lookup should not hit the network")
	}

	// Available should be true once a successful fetch has happened.
	if !d.Available() {
		t.Error("Available should report true after successful fetch")
	}
	names := d.Names()
	if len(names) == 0 {
		t.Error("Names should report discovered entries")
	}
}

func TestProfileDiscovererInvalidateTriggersReload(t *testing.T) {
	var profileHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			atomic.AddInt32(&profileHits, 1)
			fmt.Fprint(w, `{"inferenceProfileSummaries":[
				{"inferenceProfileId":"us.anthropic.claude-opus-4-7-20260115-v1:0","status":"ACTIVE"}
			]}`)
		default:
			fmt.Fprint(w, `{"modelSummaries":[]}`)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	if _, ok := d.Lookup(context.Background(), "claude-opus-4-7"); !ok {
		t.Fatal("initial lookup failed")
	}
	if atomic.LoadInt32(&profileHits) != 1 {
		t.Fatalf("expected 1 profile hit, got %d", profileHits)
	}

	// Invalidate is rate-limited against recent fetches to prevent AWS
	// control-plane amplification — call it before the min interval to
	// confirm the guard, then again after clearing lastFetchAt.
	d.Invalidate()
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&profileHits) != 1 {
		t.Errorf("Invalidate within rate-limit window should be a no-op; got %d hits", profileHits)
	}

	d.mu.Lock()
	d.lastFetchAt = time.Time{}
	d.mu.Unlock()

	d.Invalidate()
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&profileHits) != 2 {
		t.Errorf("Invalidate after rate-limit window should force a reload; got %d hits", profileHits)
	}
}

func TestProfileDiscovererBacksOffOnError(t *testing.T) {
	var profileHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/inference-profiles" {
			atomic.AddInt32(&profileHits, 1)
		}
		http.Error(w, `{"message":"User is not authorized to perform bedrock:ListInferenceProfiles"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	if _, ok := d.Lookup(context.Background(), "claude-opus-4-7"); ok {
		t.Error("expected miss on failed discovery")
	}
	// Second call within failure TTL must not re-hit the network.
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&profileHits) != 1 {
		t.Errorf("failed discovery should back off; got %d hits", profileHits)
	}

	d.mu.Lock()
	wantMin := time.Now()
	wantMax := time.Now().Add(discoveryFailureTTL + time.Second)
	exp := d.expiresAt
	d.mu.Unlock()
	if exp.Before(wantMin) || exp.After(wantMax) {
		t.Errorf("unexpected backoff expiry %v", exp)
	}

	if d.Available() {
		t.Error("Available should report false when last fetch failed")
	}
}

func TestProfileDiscovererIndexesDatedNamesAndPreservesVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			fmt.Fprint(w, `{"inferenceProfileSummaries":[
				{"inferenceProfileId":"us.anthropic.claude-3-5-sonnet-20241022-v2:0","status":"ACTIVE"},
				{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
			]}`)
		default:
			fmt.Fprint(w, `{"modelSummaries":[]}`)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	// Dated-name lookup must carry the upstream's real version suffix (v2:0
	// for 3.5-sonnet), not a locally-fabricated v1:0 — this is the
	// regression the old versionOverrides table used to paper over.
	id, ok := d.Lookup(context.Background(), "claude-3-5-sonnet-20241022")
	if !ok || id != "us.anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Errorf("dated name lookup got (%q,%v); want the v2:0 profile", id, ok)
	}

	// Short-name lookup still works.
	id, ok = d.Lookup(context.Background(), "claude-opus-4-5")
	if !ok || id != "us.anthropic.claude-opus-4-5-20251101-v1:0" {
		t.Errorf("short-name lookup got (%q,%v)", id, ok)
	}

	// Operator-facing Names() should not leak the dated index entries.
	for _, n := range d.Names() {
		if modelDatePattern.MatchString(n) {
			t.Errorf("Names() leaked dated entry: %q", n)
		}
	}
}

func TestProfileDiscovererConcurrentColdStartBlocksOnSingleFlight(t *testing.T) {
	var profileHits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			atomic.AddInt32(&profileHits, 1)
			<-release // hold the first profile request until callers have queued
			fmt.Fprint(w, `{"inferenceProfileSummaries":[
				{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
			]}`)
		default:
			fmt.Fprint(w, `{"modelSummaries":[]}`)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	const N = 8
	results := make(chan bool, N)
	for i := 0; i < N; i++ {
		go func() {
			_, ok := d.Lookup(context.Background(), "claude-opus-4-5")
			results <- ok
		}()
	}

	// Wait for the first fetch to start, then let it finish.
	for atomic.LoadInt32(&profileHits) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	close(release)

	// Every concurrent caller must see the hit — the old "loading=true →
	// early return" path would have let the other 7 miss.
	for i := 0; i < N; i++ {
		if !<-results {
			t.Error("concurrent Lookup saw miss during single-flight load")
		}
	}

	// Only one upstream profile fetch should have fired.
	if got := atomic.LoadInt32(&profileHits); got != 1 {
		t.Errorf("expected single upstream profile fetch, got %d", got)
	}
}

func TestExtractNameAndDateAcceptsFoundationModels(t *testing.T) {
	cases := []struct {
		id        string
		wantShort string
		wantDate  string
		wantOK    bool
	}{
		// Foundation models (undated — newer AWS releases).
		{"anthropic.claude-sonnet-4-6", "claude-sonnet-4-6", "", true},
		{"anthropic.claude-opus-4-6-v1", "claude-opus-4-6", "", true},
		{"anthropic.claude-haiku-4-5", "claude-haiku-4-5", "", true},
		// Foundation model shape mustn't swallow the region prefix.
		{"us.anthropic.claude-sonnet-4-6", "", "", false},
		// Legacy "claude-v2": the `2` satisfies the digit requirement
		// and the whole name is preserved — clients still send it.
		{"anthropic.claude-v2", "claude-v2", "", true},
		// Inference-profile shape still works.
		{"us.anthropic.claude-opus-4-7-20260115-v1:0", "claude-opus-4-7", "20260115", true},
	}
	for _, c := range cases {
		short, date, ok := extractNameAndDate(c.id)
		if ok != c.wantOK || short != c.wantShort || date != c.wantDate {
			t.Errorf("extractNameAndDate(%q) = (%q,%q,%v); want (%q,%q,%v)",
				c.id, short, date, ok, c.wantShort, c.wantDate, c.wantOK)
		}
	}
}

func TestProfileDiscovererMergesFoundationModelsAndProfilesWin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			// claude-opus-4-5 has both profile and FM — profile must win.
			fmt.Fprint(w, `{"inferenceProfileSummaries":[
				{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
			]}`)
		case "/foundation-models":
			fmt.Fprint(w, `{"modelSummaries":[
				{"modelId":"anthropic.claude-opus-4-5-20251101-v1:0","modelLifecycle":{"status":"ACTIVE"}},
				{"modelId":"anthropic.claude-sonnet-4-6","modelLifecycle":{"status":"ACTIVE"}},
				{"modelId":"anthropic.claude-opus-4-6-v1","modelLifecycle":{"status":"ACTIVE"}},
				{"modelId":"anthropic.claude-v2","modelLifecycle":{"status":"ACTIVE"}}
			]}`)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	// Profile wins for claude-opus-4-5 — the returned ID must be the
	// region-prefixed profile form, not the bare anthropic.X foundation ID.
	id, ok := d.Lookup(context.Background(), "claude-opus-4-5")
	if !ok {
		t.Fatalf("expected claude-opus-4-5 hit")
	}
	if id != "us.anthropic.claude-opus-4-5-20251101-v1:0" {
		t.Errorf("profile must win over foundation model for same short name; got %q", id)
	}

	// Foundation-model-only release: must resolve to the bare anthropic.X ID
	// (never with a region prefix — that produces invalid Bedrock targets).
	id, ok = d.Lookup(context.Background(), "claude-sonnet-4-6")
	if !ok || id != "anthropic.claude-sonnet-4-6" {
		t.Errorf("claude-sonnet-4-6 got (%q,%v); want (anthropic.claude-sonnet-4-6,true)", id, ok)
	}

	// -v1 suffix on foundation IDs is preserved verbatim.
	id, ok = d.Lookup(context.Background(), "claude-opus-4-6")
	if !ok || id != "anthropic.claude-opus-4-6-v1" {
		t.Errorf("claude-opus-4-6 got (%q,%v); want (anthropic.claude-opus-4-6-v1,true)", id, ok)
	}

	// Legacy "claude-v2" should be indexed verbatim (real Bedrock ID).
	if id, ok := d.Lookup(context.Background(), "claude-v2"); !ok || id != "anthropic.claude-v2" {
		t.Errorf("claude-v2 got (%q,%v); want (anthropic.claude-v2,true)", id, ok)
	}
	if _, ok := d.Lookup(context.Background(), "claude"); ok {
		t.Error("bare 'claude' (no version digit) must not be indexed")
	}
}

func TestProfileDiscovererEntriesTracksSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			fmt.Fprint(w, `{"inferenceProfileSummaries":[
				{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
			]}`)
		case "/foundation-models":
			// claude-opus-4-5 overlap — profile must win AND be labeled
			// inference-profile. claude-sonnet-4-6 is FM-only. And
			// anthropic.claude-3-5-sonnet-20241022-v2:0 is a dated
			// profile-shaped ID but coming from the FM catalog, so it
			// must be labeled foundation-model (not inferred from shape).
			fmt.Fprint(w, `{"modelSummaries":[
				{"modelId":"anthropic.claude-opus-4-5-20251101-v1:0","modelLifecycle":{"status":"ACTIVE"}},
				{"modelId":"anthropic.claude-sonnet-4-6","modelLifecycle":{"status":"ACTIVE"}},
				{"modelId":"anthropic.claude-3-5-sonnet-20241022-v2:0","modelLifecycle":{"status":"ACTIVE"}}
			]}`)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)
	// Force a load.
	_, _ = d.Lookup(context.Background(), "__force_load__")

	byName := map[string]Entry{}
	for _, e := range d.Entries() {
		byName[e.ShortName] = e
	}

	if got := byName["claude-opus-4-5"].Source; got != "inference-profile" {
		t.Errorf("overlap short name should keep inference-profile source; got %q", got)
	}
	if got := byName["claude-sonnet-4-6"].Source; got != "foundation-model" {
		t.Errorf("FM-only should be labeled foundation-model; got %q", got)
	}
	if got := byName["claude-3-5-sonnet"].Source; got != "foundation-model" {
		t.Errorf("dated FM entry must be labeled foundation-model even though its shape resembles a profile; got %q", got)
	}
}

func TestProfileDiscovererPartialFailure(t *testing.T) {
	// ListInferenceProfiles errors (e.g. missing IAM), but foundation
	// models still resolve — discovery should surface what it can.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/inference-profiles":
			http.Error(w, `{"message":"not authorized"}`, http.StatusForbidden)
		case "/foundation-models":
			fmt.Fprint(w, `{"modelSummaries":[
				{"modelId":"anthropic.claude-sonnet-4-6","modelLifecycle":{"status":"ACTIVE"}}
			]}`)
		}
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	id, ok := d.Lookup(context.Background(), "claude-sonnet-4-6")
	if !ok || id != "anthropic.claude-sonnet-4-6" {
		t.Errorf("partial discovery must still return FM hits; got (%q,%v)", id, ok)
	}
}

func TestNewerProfileHandlesDoubleDigitVersion(t *testing.T) {
	// -v10:0 is "older" lexicographically than -v9:0; must compare numerically.
	older := "us.anthropic.claude-opus-4-5-20251101-v9:0"
	newer := "us.anthropic.claude-opus-4-5-20251101-v10:0"
	if !newerProfile(newer, older) {
		t.Errorf("newerProfile(v10, v9) must be true")
	}
	if newerProfile(older, newer) {
		t.Errorf("newerProfile(v9, v10) must be false")
	}
}

func TestProfileDiscovererContextCancelDoesNotPoisonCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the caller cancels so we always observe context.Canceled.
		<-r.Context().Done()
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call so fetch returns context.Canceled

	if _, ok := d.Lookup(ctx, "claude-opus-4-7"); ok {
		t.Error("expected miss on cancelled lookup")
	}

	// Cache must NOT be marked with a failure TTL — a transient client cancel
	// should not back off the shared cache for other callers.
	d.mu.Lock()
	lastErr := d.lastErr
	expiresAt := d.expiresAt
	d.mu.Unlock()
	if lastErr != nil {
		t.Errorf("lastErr should remain nil after ctx cancel, got %v", lastErr)
	}
	if !expiresAt.IsZero() {
		t.Errorf("expiresAt should remain zero after ctx cancel, got %v", expiresAt)
	}
}

// newDiscovererForTest builds a discoverer whose fetch() call is redirected
// to the provided test server URL. We swap the base URL via a transport that
// rewrites the Host so SigV4 still passes for bedrock.us-east-1.amazonaws.com.
func newDiscovererForTest(targetURL string) *profileDiscoverer {
	target := strings.TrimPrefix(strings.TrimPrefix(targetURL, "http://"), "https://")
	client := &http.Client{
		Transport: &redirectTransport{target: target},
	}
	creds := credentials.NewStaticCredentialsProvider("AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "")
	d := newProfileDiscoverer(client, creds, "us-east-1")
	d.ttl = 10 * time.Minute
	return d
}

// redirectTransport rewrites every request to point at the httptest server
// while leaving the original Host header intact so SigV4 signature matches.
type redirectTransport struct{ target string }

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.target
	return http.DefaultTransport.RoundTrip(req)
}
