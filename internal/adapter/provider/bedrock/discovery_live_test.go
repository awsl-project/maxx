package bedrock

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
)

// TestLiveDiscovery hits the real AWS bedrock:ListInferenceProfiles API.
// Guarded by MAXX_BEDROCK_LIVE=1 so CI and normal `go test ./...` skip it.
// Reads creds from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_REGION.
func TestLiveDiscovery(t *testing.T) {
	if os.Getenv("MAXX_BEDROCK_LIVE") != "1" {
		t.Skip("set MAXX_BEDROCK_LIVE=1 to run against real AWS")
	}
	ak := os.Getenv("AWS_ACCESS_KEY_ID")
	sk := os.Getenv("AWS_SECRET_ACCESS_KEY")
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	if ak == "" || sk == "" {
		t.Fatal("missing AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY")
	}

	creds := credentials.NewStaticCredentialsProvider(ak, sk, "")
	d := newProfileDiscoverer(newHTTPClient(), creds, region)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Force a load via an arbitrary lookup (miss is fine — we just want fetch).
	_, _ = d.Lookup(ctx, "__force_load__")

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastErr != nil {
		t.Fatalf("discovery failed: %v", d.lastErr)
	}
	if len(d.entries) == 0 {
		t.Fatal("no entries discovered")
	}

	names := make([]string, 0, len(d.entries))
	for k := range d.entries {
		names = append(names, k)
	}
	sort.Strings(names)
	t.Logf("region=%s discovered %d Anthropic profiles:", region, len(d.entries))
	for _, n := range names {
		t.Logf("  %-30s -> %s (%s)", n, d.entries[n].id, d.entries[n].source.label())
	}

	// Regression pin: Claude 4.x FM-only entries are listed by AWS with
	// inferenceTypesSupported=["INFERENCE_PROFILE"]. If the filter in
	// fetchFoundationModels regressed, one of these would be indexed as a
	// bare foundation-model ID and InvokeModel would fail at runtime with
	// "on-demand throughput isn't supported".
	for _, short := range []string{"claude-sonnet-4-6", "claude-opus-4-6"} {
		e, ok := d.entries[short]
		if !ok {
			continue
		}
		if e.source == sourceFoundation {
			t.Errorf("%s was indexed as foundation-model (%s) — filter must skip non-ON_DEMAND FM entries", short, e.id)
		}
	}
}
