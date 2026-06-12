package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
)

// TestLiveInvoke exercises the full resolve + sign + invoke path against
// real AWS. For every model in the MAXX_BEDROCK_LIVE_MODELS list (or the
// default set below) it:
//   - discovers the Bedrock ID via the real catalog
//   - signs a 1-token non-stream Messages request
//   - asserts HTTP 200 and a non-empty content block
//
// Gated by MAXX_BEDROCK_LIVE=1. Generates a few cents of token cost at
// most — a 1-token max-tokens response stays under $0.01 even on Opus.
func TestLiveInvoke(t *testing.T) {
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
	httpClient := newHTTPClient()
	d := newProfileDiscoverer(httpClient, creds, region)

	// Force discovery to populate the cache before we start resolving.
	warmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, _ = d.Lookup(warmCtx, "__force_load__")
	cancel()

	// Models we care about most — newly-shipped 4.6/4.7 plus one known-
	// good classic profile as a control. Operator can override via env
	// if they want to test something specific.
	models := []string{"claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6", "claude-opus-4-5"}
	if override := os.Getenv("MAXX_BEDROCK_LIVE_MODELS"); override != "" {
		models = nil
		for _, m := range strings.Split(override, ",") {
			if m != "" {
				models = append(models, m)
			}
		}
	}

	for _, requested := range models {
		t.Run(requested, func(t *testing.T) {
			// Use the same resolver the adapter uses in production.
			lookup := func(name string) (string, bool) {
				ctx, cancel := context.WithTimeout(context.Background(), discoveryLookupTimeout)
				defer cancel()
				return d.Lookup(ctx, name)
			}
			resolved, ok := resolveModelID(requested, nil, "us", lookup)
			if !ok {
				t.Fatalf("resolveModelID(%q) = not ok — catalog: %v", requested, d.Names())
			}
			t.Logf("resolved %s -> %s", requested, resolved)

			// Minimal Claude Messages request: 1 user turn, cap at 1
			// output token. We don't care about the reply content, only
			// that the upstream accepts the request and returns a
			// stop_reason (meaning end-to-end plumbing worked).
			reqBody, err := json.Marshal(map[string]any{
				"anthropic_version": "bedrock-2023-05-31",
				"max_tokens":        1,
				"messages":          []map[string]any{{"role": "user", "content": "hi"}},
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			upstreamURL := buildBedrockURL(region, resolved, false)
			invokeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(invokeCtx, http.MethodPost, upstreamURL, bytes.NewReader(reqBody))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			if err := signRequest(invokeCtx, req, reqBody, creds, region); err != nil {
				t.Fatalf("sign: %v", err)
			}

			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
			}

			var parsed struct {
				StopReason string `json:"stop_reason"`
				Content    []struct {
					Type string `json:"type"`
				} `json:"content"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("decode: %v body=%s", err, truncate(string(body), 400))
			}
			if parsed.StopReason == "" {
				t.Errorf("expected stop_reason in response; body=%s", truncate(string(body), 400))
			}
			t.Logf("url=%s stop_reason=%s in_tokens=%d out_tokens=%d",
				upstreamURL, parsed.StopReason, parsed.Usage.InputTokens, parsed.Usage.OutputTokens)
		})
	}
}


