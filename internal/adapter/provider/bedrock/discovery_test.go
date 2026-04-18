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
		{"anthropic.claude-v2", "", false}, // no date
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		token := r.URL.Query().Get("nextToken")
		switch token {
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
	before := atomic.LoadInt32(&hits)
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&hits)-before > 0 {
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"inferenceProfileSummaries":[
			{"inferenceProfileId":"us.anthropic.claude-opus-4-7-20260115-v1:0","status":"ACTIVE"}
		]}`)
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	if _, ok := d.Lookup(context.Background(), "claude-opus-4-7"); !ok {
		t.Fatal("initial lookup failed")
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", hits)
	}

	// Invalidate is rate-limited against recent fetches to prevent AWS
	// control-plane amplification — call it before the min interval to
	// confirm the guard, then again after clearing lastFetchAt.
	d.Invalidate()
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("Invalidate within rate-limit window should be a no-op; got %d hits", hits)
	}

	d.mu.Lock()
	d.lastFetchAt = time.Time{}
	d.mu.Unlock()

	d.Invalidate()
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("Invalidate after rate-limit window should force a reload; got %d hits", hits)
	}
}

func TestProfileDiscovererBacksOffOnError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, `{"message":"User is not authorized to perform bedrock:ListInferenceProfiles"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	d := newDiscovererForTest(srv.URL)

	if _, ok := d.Lookup(context.Background(), "claude-opus-4-7"); ok {
		t.Error("expected miss on failed discovery")
	}
	// Second call within failure TTL must not re-hit the network.
	_, _ = d.Lookup(context.Background(), "claude-opus-4-7")
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("failed discovery should back off; got %d hits", hits)
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
		fmt.Fprint(w, `{"inferenceProfileSummaries":[
			{"inferenceProfileId":"us.anthropic.claude-3-5-sonnet-20241022-v2:0","status":"ACTIVE"},
			{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
		]}`)
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
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-release // hold the first request until all concurrent callers have queued
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"inferenceProfileSummaries":[
			{"inferenceProfileId":"us.anthropic.claude-opus-4-5-20251101-v1:0","status":"ACTIVE"}
		]}`)
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
	for atomic.LoadInt32(&hits) == 0 {
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

	// Only one upstream fetch should have fired.
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected single upstream fetch, got %d", got)
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
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
