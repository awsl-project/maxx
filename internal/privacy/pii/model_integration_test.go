package pii

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Real model verification is explicit: regular CI uses the upstream fake helper
// and tokenizer/kernel fixtures, and never downloads gigabytes on each PR.
func TestRealLocalModels(t *testing.T) {
	models := os.Getenv("MAXX_PRIVACY_TEST_MODELS")
	if models == "" {
		t.Skip("set MAXX_PRIVACY_TEST_MODELS to explicit model IDs for real inference")
	}
	for _, id := range strings.Split(models, ",") {
		t.Run(id, func(t *testing.T) {
			if id != AstrLinkGuard && id != PrivacyFilter {
				t.Fatalf("unknown test model %s", id)
			}
			model := Of(id)
			if !model.Supported() {
				t.Skip("model backend is not available on this platform")
			}
			defer model.Stop()
			defer model.Cancel()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
			defer cancel()
			if !model.Installed() {
				if err := model.Download(func() {}); err != nil {
					t.Fatal(err)
				}
				for !model.Installed() {
					status := model.Now()
					if status.Error != "" {
						t.Fatalf("model download: %s", status.Error)
					}
					select {
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					case <-time.After(250 * time.Millisecond):
					}
				}
			}
			texts := []string{"Contact Alice Zhang at alice@example.com or call 13800138000.", "The password is ExampleSecret1234, keep it private."}
			found, err := model.Find(ctx, texts)
			if err != nil || len(found) != len(texts) {
				t.Fatalf("real inference: %v, results=%d", err, len(found))
			}
			n := 0
			for i, spans := range found {
				for _, span := range spans {
					if span.Start < 0 || span.End <= span.Start || span.End > len(texts[i]) || span.Score < Floor || span.Score > 1 {
						t.Fatalf("invalid real-model span: %+v", span)
					}
					n++
				}
			}
			if n == 0 {
				t.Fatal("real model did not find any synthetic private content")
			}
			t.Logf("real %s backend=%s, valid spans=%d", id, model.Now().Device, n)
		})
	}
}
