package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

// foundationModelPattern extracts the short name from a Bedrock foundation-
// model ID that has no release date (newer AWS releases frequently omit
// it). Shapes:
//
//	anthropic.claude-sonnet-4-6
//	anthropic.claude-opus-4-6-v1
//
// The capture requires at least one digit to reject ancient legacy IDs
// like "anthropic.claude-v2" that would otherwise collapse to "claude".
var foundationModelPattern = regexp.MustCompile(
	`^anthropic\.(claude-[a-z0-9-]*?\d[a-z0-9-]*?)(?:-v\d+)?$`,
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
// Set well above the observed p99 (~5s for two paginated calls) so
// concurrent cold-start callers waiting on single-flight don't prematurely
// fall through on tail-latency spikes.
const discoveryLookupTimeout = 20 * time.Second

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
// entrySource is the AWS catalog a discovered entry came from. Tracked
// at ingestion time because the source cannot always be recovered from
// the ID shape alone (foundation-model IDs can carry the same date +
// version suffix as inference-profile bases minus the region prefix).
type entrySource uint8

const (
	sourceFoundation entrySource = iota
	sourceInferenceProfile
)

// discoveredEntry pairs an invoke-ready Bedrock ID with the catalog
// that produced it.
type discoveredEntry struct {
	id     string
	source entrySource
}

type profileDiscoverer struct {
	httpClient *http.Client
	creds      credentials.StaticCredentialsProvider
	region     string
	ttl        time.Duration

	mu          sync.Mutex
	entries     map[string]discoveredEntry
	expiresAt   time.Time
	lastFetchAt time.Time
	lastErr     error
	everLoaded  bool
	loadingCh   chan struct{}
}

func newProfileDiscoverer(httpClient *http.Client, creds credentials.StaticCredentialsProvider, region string) *profileDiscoverer {
	return &profileDiscoverer{
		httpClient: httpClient,
		creds:      creds,
		region:     region,
		ttl:        defaultDiscoveryTTL,
		entries:    map[string]discoveredEntry{},
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
	e, ok := d.entries[shortName]
	if !ok {
		return "", false
	}
	return e.id, true
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

// Entry pairs an Anthropic short name with the invoke-ready Bedrock ID
// discovery resolved it to, plus the AWS catalog ("inference-profile"
// or "foundation-model") that produced it. Used by the admin API to
// surface "what can this provider actually call right now" — and to
// tell the two catalogs apart — to operators in the UI.
type Entry struct {
	ShortName string
	BedrockID string
	Source    string // "inference-profile" or "foundation-model"
}

// Entries returns the short-name → Bedrock-ID pairs discovery is serving
// for this provider, excluding the internal dated-name index entries.
func (d *profileDiscoverer) Entries() []Entry {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Entry, 0, len(d.entries))
	for k, v := range d.entries {
		if modelDatePattern.MatchString(k) {
			continue
		}
		out = append(out, Entry{ShortName: k, BedrockID: v.id, Source: v.source.label()})
	}
	return out
}

func (s entrySource) label() string {
	if s == sourceInferenceProfile {
		return "inference-profile"
	}
	return "foundation-model"
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
	now := time.Now()
	switch {
	case err == nil:
		d.lastErr = nil
		if entries != nil {
			d.entries = entries
		}
		d.expiresAt = now.Add(d.ttl)
		d.lastFetchAt = now
		d.everLoaded = true
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		// Transient cancellation from a caller-supplied context — do not
		// poison the cache with a failure TTL, the next caller should retry
		// immediately rather than wait out discoveryFailureTTL.
	default:
		// Keep previous entries (if any) but back off before retrying.
		d.lastErr = err
		d.expiresAt = now.Add(discoveryFailureTTL)
		d.lastFetchAt = now
	}
	d.mu.Unlock()
}

// fetch builds the Anthropic short-name → Bedrock-ID map for the configured
// region. Two upstream catalogs contribute:
//
//   - ListInferenceProfiles: preferred. Cross-region profiles (e.g.
//     us.anthropic.claude-opus-4-5-20251101-v1:0) give automatic regional
//     failover and are AWS's recommended invocation path for Anthropic.
//   - ListFoundationModels: fallback. Models that AWS has released but not
//     (yet) fronted with an inference profile — most recent example is
//     Claude Sonnet 4.6 / Opus 4.6 which exist only as foundation models
//     in every region. Without this call, those models are invisible to
//     discovery even though they work when invoked directly.
//
// When the same short name appears in both catalogs the profile wins.
func (d *profileDiscoverer) fetch(ctx context.Context) (map[string]discoveredEntry, error) {
	profiles, profErr := d.fetchInferenceProfiles(ctx)
	foundations, fmErr := d.fetchFoundationModels(ctx)

	// If both fail, propagate the profiles error (the primary source).
	if profErr != nil && fmErr != nil {
		return nil, profErr
	}
	// Log partial failures loudly — silently running with only one of the
	// two catalogs can mask a real IAM / config issue (e.g. missing
	// bedrock:ListInferenceProfiles permission), and cross-region routing
	// would silently downgrade to whichever region the current endpoint
	// serves. Logging lets operators notice without breaking requests.
	if profErr != nil {
		log.Printf("bedrock discovery: inference profiles unavailable in %s (%v); falling back to foundation models only", d.region, profErr)
	}
	if fmErr != nil {
		log.Printf("bedrock discovery: foundation models unavailable in %s (%v); falling back to inference profiles only", d.region, fmErr)
	}
	merged := make(map[string]discoveredEntry, len(profiles)+len(foundations))
	for k, v := range foundations {
		merged[k] = discoveredEntry{id: v, source: sourceFoundation}
	}
	for k, v := range profiles {
		// Profile wins over foundation for the same short name — and
		// we relabel the source even if a foundation entry already sat
		// under this key, because the chosen invoke target is now the
		// profile's.
		merged[k] = discoveredEntry{id: v, source: sourceInferenceProfile}
	}
	return merged, nil
}

func (d *profileDiscoverer) fetchInferenceProfiles(ctx context.Context) (map[string]string, error) {
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

		body, err := d.signedGet(ctx, fullURL)
		if err != nil {
			return nil, err
		}

		var parsed struct {
			InferenceProfileSummaries []struct {
				InferenceProfileID string `json:"inferenceProfileId"`
				Status             string `json:"status"`
			} `json:"inferenceProfileSummaries"`
			NextToken string `json:"nextToken"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode inference profiles: %w", err)
		}

		for _, s := range parsed.InferenceProfileSummaries {
			if s.Status != "" && s.Status != "ACTIVE" {
				continue
			}
			short, date, ok := extractNameAndDate(s.InferenceProfileID)
			if !ok {
				continue
			}
			// Index under the short name ("claude-3-5-sonnet") and, when
			// a date is present, also the dated name ("claude-3-5-sonnet-
			// 20241022"). Dated-name indexing lets a client request a
			// specific release + version suffix (e.g. Bedrock's v2:0 for
			// 3.5-sonnet) without hardcoding version overrides locally.
			upsertEntry(entries, short, s.InferenceProfileID)
			if date != "" {
				upsertEntry(entries, short+"-"+date, s.InferenceProfileID)
			}
		}

		if parsed.NextToken == "" {
			break
		}
		nextToken = parsed.NextToken
	}

	return entries, nil
}

func (d *profileDiscoverer) fetchFoundationModels(ctx context.Context) (map[string]string, error) {
	entries := map[string]string{}
	q := url.Values{}
	q.Set("byProvider", "anthropic")
	fullURL := fmt.Sprintf("https://bedrock.%s.amazonaws.com/foundation-models?%s", d.region, q.Encode())

	body, err := d.signedGet(ctx, fullURL)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		ModelSummaries []struct {
			ModelID        string `json:"modelId"`
			ModelLifecycle struct {
				Status string `json:"status"`
			} `json:"modelLifecycle"`
		} `json:"modelSummaries"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode foundation models: %w", err)
	}

	for _, s := range parsed.ModelSummaries {
		if s.ModelLifecycle.Status != "" && s.ModelLifecycle.Status != "ACTIVE" {
			continue
		}
		short, date, ok := extractNameAndDate(s.ModelID)
		if !ok {
			continue
		}
		upsertEntry(entries, short, s.ModelID)
		if date != "" {
			upsertEntry(entries, short+"-"+date, s.ModelID)
		}
	}
	return entries, nil
}

// signedGet issues a SigV4-signed GET and returns the body. Non-2xx becomes
// an error containing a truncated body so failures stay debuggable without
// leaking full AWS error payloads into our own error chain.
func (d *profileDiscoverer) signedGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	if err := signRequest(ctx, req, nil, d.creds, d.region); err != nil {
		return nil, fmt.Errorf("sign discovery request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list discovery: %w", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read discovery response: %w", readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("discovery returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// upsertEntry inserts id at key, keeping the newer ID on collisions. Called
// from both inference-profile and foundation-model loops; within one source
// the newer-wins rule via newerProfile deduplicates AWS returning multiple
// dates for the same short name.
func upsertEntry(m map[string]string, key, id string) {
	if existing, ok := m[key]; !ok || newerProfile(id, existing) {
		m[key] = id
	}
}

// extractNameAndDate returns the Anthropic short name (and release date
// when present) for a Bedrock ID — either an inference profile like
// "us.anthropic.claude-opus-4-7-20260115-v1:0" or an undated foundation
// model like "anthropic.claude-sonnet-4-6". Returns ok=false when the
// shape isn't recognised (non-Anthropic, malformed, or an ancient legacy
// name without any version digits).
func extractNameAndDate(modelID string) (short, date string, ok bool) {
	if m := profileIDPattern.FindStringSubmatch(modelID); len(m) >= 3 {
		return m[1], m[2], true
	}
	if m := foundationModelPattern.FindStringSubmatch(modelID); len(m) >= 2 {
		return m[1], "", true
	}
	return "", "", false
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
		// Defensive: callers always pass IDs already validated by
		// profileIDPattern, so this branch should be unreachable. If an
		// odd shape ever slips through, prefer deterministic ordering
		// over a crash.
		return a > b
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
