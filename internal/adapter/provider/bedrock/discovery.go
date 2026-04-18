package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

// profileDiscoverer calls bedrock:ListInferenceProfiles for a given region +
// credentials, extracts Anthropic short-name → full-profile-ID mappings via
// profileIDPattern, and caches them with a TTL. Safe for concurrent use.
//
// Design notes:
//   - The cache holds *both* successful and failed loads (with different TTLs)
//     to avoid stampedes against a broken IAM configuration.
//   - Invalidate() flips the expiry so the next Lookup reloads synchronously.
//   - A single in-flight refresh is enforced via `loading`; concurrent callers
//     read whatever state exists until the refresh completes.
type profileDiscoverer struct {
	httpClient *http.Client
	creds      credentials.StaticCredentialsProvider
	region     string
	ttl        time.Duration

	mu        sync.RWMutex
	entries   map[string]string
	expiresAt time.Time
	lastErr   error
	loading   bool
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
// like "claude-opus-4-7". It triggers a synchronous refresh if the cache is
// stale. Returns (id, true) on hit; ("", false) on miss or when discovery
// failed — callers should then fall through to the static alias table.
func (d *profileDiscoverer) Lookup(ctx context.Context, shortName string) (string, bool) {
	d.ensureFresh(ctx)

	d.mu.RLock()
	defer d.mu.RUnlock()
	id, ok := d.entries[shortName]
	return id, ok
}

// Available reports whether the last discovery attempt succeeded. Callers
// use this to distinguish "model truly not on Bedrock" (Available=true,
// Lookup miss) from "discovery never ran or failed" (Available=false).
func (d *profileDiscoverer) Available() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastErr == nil && !d.expiresAt.IsZero()
}

// Names returns every discovered short name, in arbitrary order. Used for
// error messages that list the alternatives when a request model can't be
// resolved.
func (d *profileDiscoverer) Names() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]string, 0, len(d.entries))
	for k := range d.entries {
		out = append(out, k)
	}
	return out
}

// Invalidate marks the cache as stale so the next Lookup forces a refresh.
// Called when the upstream returns a model-identifier error — AWS may have
// just rotated legacy IDs.
func (d *profileDiscoverer) Invalidate() {
	d.mu.Lock()
	d.expiresAt = time.Time{}
	d.mu.Unlock()
}

func (d *profileDiscoverer) ensureFresh(ctx context.Context) {
	d.mu.RLock()
	fresh := time.Now().Before(d.expiresAt)
	loading := d.loading
	d.mu.RUnlock()
	if fresh || loading {
		return
	}

	d.mu.Lock()
	if time.Now().Before(d.expiresAt) || d.loading {
		d.mu.Unlock()
		return
	}
	d.loading = true
	d.mu.Unlock()

	entries, err := d.fetch(ctx)

	d.mu.Lock()
	d.loading = false
	d.lastErr = err
	if err == nil {
		d.entries = entries
		d.expiresAt = time.Now().Add(d.ttl)
	} else {
		// Keep previous entries (if any) but back off before retrying.
		d.expiresAt = time.Now().Add(discoveryFailureTTL)
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
			short, ok := extractShortName(s.InferenceProfileID)
			if !ok {
				continue
			}
			// If multiple profiles collapse to the same short name, keep the
			// lexicographically greatest ID — the date suffix makes that the
			// newest dated release.
			if existing, exists := entries[short]; !exists || s.InferenceProfileID > existing {
				entries[short] = s.InferenceProfileID
			}
		}

		if parsed.NextToken == "" {
			break
		}
		nextToken = parsed.NextToken
	}

	return entries, nil
}

// extractShortName returns ("claude-opus-4-7", true) for
// "us.anthropic.claude-opus-4-7-20260115-v1:0"; ok=false for anything else.
func extractShortName(profileID string) (string, bool) {
	m := profileIDPattern.FindStringSubmatch(profileID)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
