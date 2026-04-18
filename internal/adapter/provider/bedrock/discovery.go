package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
)

// profileIDPattern extracts the Anthropic short name + date from a Bedrock
// inference profile ID. Supported shapes:
//
//	us.anthropic.claude-opus-4-7-20260115-v1:0
//	anthropic.claude-3-5-sonnet-20241022-v2:0
//	eu.anthropic.claude-sonnet-4-5-20250514-v1:0
//
// Capture groups: 1=short name (e.g. "claude-opus-4-7"), 2=date (YYYYMMDD).
var profileIDPattern = regexp.MustCompile(
	`^(?:[a-z]{2,}\.)?anthropic\.(claude-[a-z0-9-]+?)-(\d{8})-v\d+:\d+$`,
)

// defaultDiscoveryTTL is how long a successful discovery result is cached.
const defaultDiscoveryTTL = 30 * time.Minute

// discoveryFailureTTL caps the wait before retrying after a failed discovery
// (e.g. missing IAM permission). Shorter than the success TTL so a fix lands
// quickly, longer than a single request so we don't hammer the API.
const discoveryFailureTTL = 2 * time.Minute

// discoveryLookupTimeout bounds a single ListInferenceProfiles round-trip
// when called during model resolution. Kept separate from the request's
// context so a client disconnect can't cancel (and poison) discovery.
const discoveryLookupTimeout = 10 * time.Second

// minInvalidateInterval rate-limits Invalidate() so a burst of upstream
// model-unavailable errors (e.g. from a misconfigured modelMapping
// pointing at a bogus ID) can't amplify into a ListInferenceProfiles
// flood against the AWS control plane.
const minInvalidateInterval = 60 * time.Second

// profileDiscoverer calls bedrock:ListInferenceProfiles for a given region +
// credentials, extracts Anthropic short-name → full-profile-ID mappings via
// profileIDPattern, and caches them with a TTL. Safe for concurrent use.
//
// Design notes:
//   - The cache holds *both* successful and failed loads (with different TTLs)
//     to avoid stampedes against a broken IAM configuration.
//   - Invalidate() flips the expiry so the next Lookup reloads synchronously,
//     but it is rate-limited by minInvalidateInterval to prevent amplification.
//   - A single in-flight refresh is coordinated via loadingCh: concurrent
//     callers block on the same load instead of each doing their own.
//   - everLoaded stays true once discovery has succeeded at least once, so
//     Available() can distinguish "fresh but stale" from "never loaded".
type profileDiscoverer struct {
	httpClient *http.Client
	creds      credentials.StaticCredentialsProvider
	region     string
	ttl        time.Duration

	mu           sync.Mutex
	entries      map[string]string
	expiresAt    time.Time
	lastFetchAt  time.Time
	lastErr      error
	everLoaded   bool
	loadingCh    chan struct{}
}

func newProfileDiscoverer(httpClient *http.Client, creds credentials.StaticCredentialsProvider, region string) *profileDiscoverer {
	return &profileDiscoverer{
		httpClient: httpClient,
		creds:      creds,
		region:     region,
		ttl:        defaultDiscoveryTTL,
		entries:    map[string]string{},
	}
}

// Lookup returns the discovered Bedrock profile ID for an Anthropic short name
// like "claude-opus-4-7". Triggers a synchronous refresh via ensureFresh when
// the cache is stale. Returns (id, true) on hit; ("", false) on miss — miss
// means either the profile genuinely isn't on Bedrock (Available() == true)
// or discovery never succeeded (Available() == false). Callers in adapter.go
// use Available() + Names() to build a precise unresolvableModelError; they
// no longer fall back to any static alias table.
func (d *profileDiscoverer) Lookup(ctx context.Context, shortName string) (string, bool) {
	d.ensureFresh(ctx)

	d.mu.Lock()
	defer d.mu.Unlock()
	id, ok := d.entries[shortName]
	return id, ok
}

// Available reports whether discovery has ever completed successfully. A
// miss after Available()==true means the model is genuinely not on Bedrock
// in this region; a miss with Available()==false means discovery never
// worked (missing IAM, etc.) and callers should say so in the error.
func (d *profileDiscoverer) Available() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.everLoaded
}

// Names returns every discovered short name, in arbitrary order. Used for
// error messages that list the alternatives when a request model can't be
// resolved.
func (d *profileDiscoverer) Names() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.entries))
	for k := range d.entries {
		// Only include pure short names in the operator-facing list; the
		// dated-name index entries are an implementation detail.
		if !modelDatePattern.MatchString(k) {
			out = append(out, k)
		}
	}
	return out
}

// Invalidate marks the cache as stale so the next Lookup forces a refresh.
// Rate-limited: if the most recent fetch completed less than
// minInvalidateInterval ago, the call is a no-op. This protects against a
// flood of upstream ModelUnavailable errors (from e.g. a bad modelMapping
// entry) amplifying into a burst of ListInferenceProfiles calls.
func (d *profileDiscoverer) Invalidate() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.lastFetchAt.IsZero() && time.Since(d.lastFetchAt) < minInvalidateInterval {
		return
	}
	d.expiresAt = time.Time{}
}

func (d *profileDiscoverer) ensureFresh(ctx context.Context) {
	d.mu.Lock()
	if time.Now().Before(d.expiresAt) {
		d.mu.Unlock()
		return
	}
	if d.loadingCh != nil {
		// Another goroutine is fetching — block on it so we either return
		// with fresh data or propagate the same failure, instead of each
		// concurrent caller seeing an empty cache.
		ch := d.loadingCh
		d.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return
	}
	ch := make(chan struct{})
	d.loadingCh = ch
	d.mu.Unlock()

	entries, err := d.fetch(ctx)

	d.mu.Lock()
	d.loadingCh = nil
	close(ch)
	switch {
	case err == nil:
		d.lastErr = nil
		d.entries = entries
		d.expiresAt = time.Now().Add(d.ttl)
		d.lastFetchAt = time.Now()
		d.everLoaded = true
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		// Transient cancellation from a caller-supplied context — do not
		// poison the cache with a failure TTL, the next caller should retry
		// immediately rather than wait out discoveryFailureTTL.
	default:
		// Keep previous entries (if any) but back off before retrying.
		d.lastErr = err
		d.expiresAt = time.Now().Add(discoveryFailureTTL)
		d.lastFetchAt = time.Now()
	}
	d.mu.Unlock()
}

// fetch calls ListInferenceProfiles and returns a shortName→profileID map.
// Handles pagination via nextToken.
func (d *profileDiscoverer) fetch(ctx context.Context) (map[string]string, error) {
	entries := map[string]string{}
	var nextToken string
	base := fmt.Sprintf("https://bedrock.%s.amazonaws.com/inference-profiles", d.region)

	for {
		q := url.Values{}
		q.Set("typeEquals", "SYSTEM_DEFINED")
		q.Set("maxResults", "100")
		if nextToken != "" {
			q.Set("nextToken", nextToken)
		}
		fullURL := base + "?" + q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build discovery request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		if err := signRequest(ctx, req, nil, d.creds, d.region); err != nil {
			return nil, fmt.Errorf("sign discovery request: %w", err)
		}

		resp, err := d.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list inference profiles: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read discovery response: %w", readErr)
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("list inference profiles returned %d: %s", resp.StatusCode, truncate(string(body), 200))
		}

		var parsed struct {
			InferenceProfileSummaries []struct {
				InferenceProfileID string `json:"inferenceProfileId"`
				Status             string `json:"status"`
			} `json:"inferenceProfileSummaries"`
			NextToken string `json:"nextToken"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode discovery response: %w", err)
		}

		for _, s := range parsed.InferenceProfileSummaries {
			if s.Status != "" && s.Status != "ACTIVE" {
				continue
			}
			short, date, ok := extractNameAndDate(s.InferenceProfileID)
			if !ok {
				continue
			}
			// Index under both the short name ("claude-3-5-sonnet") and the
			// dated name ("claude-3-5-sonnet-20241022"). Dated-name indexing
			// lets a client request a specific release + version suffix
			// (e.g. Bedrock's v2:0 for 3.5-sonnet) without us ever having to
			// hardcode version overrides locally.
			dated := short + "-" + date
			// Short name: newest date wins.
			if existing, exists := entries[short]; !exists || newerProfile(s.InferenceProfileID, existing) {
				entries[short] = s.InferenceProfileID
			}
			// Dated name: highest version suffix wins for the same date.
			if existing, exists := entries[dated]; !exists || newerProfile(s.InferenceProfileID, existing) {
				entries[dated] = s.InferenceProfileID
			}
		}

		if parsed.NextToken == "" {
			break
		}
		nextToken = parsed.NextToken
	}

	return entries, nil
}

// extractNameAndDate returns ("claude-opus-4-7", "20260115", true) for
// "us.anthropic.claude-opus-4-7-20260115-v1:0"; ok=false for anything that
// doesn't match the Anthropic inference-profile shape.
func extractNameAndDate(profileID string) (short, date string, ok bool) {
	m := profileIDPattern.FindStringSubmatch(profileID)
	if len(m) < 3 {
		return "", "", false
	}
	return m[1], m[2], true
}

// extractShortName is kept for tests that only care about the short name.
func extractShortName(profileID string) (string, bool) {
	short, _, ok := extractNameAndDate(profileID)
	return short, ok
}

// newerProfile returns true when a should replace b for the same indexed
// key. Dates are compared numerically via the YYYYMMDD capture; same-date
// collisions fall back to the higher version suffix ("v2:0" > "v1:0"),
// parsed numerically so "v10:0" wins over "v9:0" too.
func newerProfile(a, b string) bool {
	aDate, aVer, aOK := profileDateVersion(a)
	bDate, bVer, bOK := profileDateVersion(b)
	if !aOK || !bOK {
		return a > b // fall back to lexicographic for unparseable inputs
	}
	if aDate != bDate {
		return aDate > bDate
	}
	return aVer > bVer
}

var profileVersionPattern = regexp.MustCompile(`-(\d{8})-v(\d+):\d+$`)

func profileDateVersion(profileID string) (dateNum, verNum int, ok bool) {
	m := profileVersionPattern.FindStringSubmatch(profileID)
	if len(m) < 3 {
		return 0, 0, false
	}
	var err error
	if dateNum, err = strconv.Atoi(m[1]); err != nil {
		return 0, 0, false
	}
	if verNum, err = strconv.Atoi(m[2]); err != nil {
		return 0, 0, false
	}
	return dateNum, verNum, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
