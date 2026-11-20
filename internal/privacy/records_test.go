package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestBypassedHistorySerializesEmptyValuesAsArray(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled, cfg.BypassProviders = true, []uint64{7}
	svc := configuredService(t, cfg)
	svc.SetDirectory(t.TempDir())
	e, rejection := svc.Prepare(context.Background(), 1, TextRequest("hello"), nil, "application/json")
	if rejection != nil || e.ResolvePlan([]uint64{7}) != nil || !e.Bypassed {
		t.Fatal("provider was not exempted", rejection)
	}
	req := &domain.ProxyRequest{ID: 1, RequestID: "bypass", StartTime: time.Now(), Status: "COMPLETED", ProviderID: 7}
	e.BeginRecord(req)
	e.CaptureOutbound(e.Body)
	e.FinishRecord(req)
	restarted := NewService(svc.settings)
	restarted.SetDirectory(svc.runtime.directory)
	for _, service := range []*Service{svc, restarted} {
		page, err := service.Records(1, RecordsQuery{})
		if err != nil || page.Total != 1 {
			t.Fatalf("missing bypass record: %+v %v", page, err)
		}
		detail, err := service.Record(1, req.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range []Record{page.Items[0], detail} {
			raw, err := json.Marshal(record)
			if err != nil || !bytes.Contains(raw, []byte(`"values":[]`)) {
				t.Fatalf("empty values must be an array: %s %v", raw, err)
			}
		}
	}
}

func TestHistoryRetainsOnlyLastAttemptedOutbound(t *testing.T) {
	for _, mode := range []string{"mask", "block", "bypass", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Enabled, cfg.Words = true, []string{"Nightjar"}
			if mode == "block" {
				cfg.Mode = "block"
			}
			if mode == "bypass" {
				cfg.BypassProviders = []uint64{7}
			}
			svc := configuredService(t, cfg)
			e, rejection := svc.Prepare(context.Background(), 1, TextRequest("Nightjar"), nil, "application/json")
			if (rejection != nil) != (mode == "block") {
				t.Fatal(rejection)
			}
			req := &domain.ProxyRequest{ID: 2, RequestID: mode, Status: "COMPLETED", ProviderID: 7}
			e.BeginRecord(req)
			want := ""
			if mode == "block" {
				req.Status = "REJECTED"
			} else {
				if rejection := e.ResolvePlan([]uint64{7}); rejection != nil {
					t.Fatal(rejection)
				}
				for _, temperature := range []string{"0.1", "0.2", "0.3"} {
					body, rejection := e.Outbound([]byte(`{"messages":[{"role":"user","content":"Nightjar"}],"temperature":` + temperature + `}`))
					if rejection != nil {
						t.Fatal(rejection)
					}
					// This routing candidate never acquired a slot.
					if temperature == "0.3" {
						continue
					}
					want = string(body)
					e.CaptureOutbound(body)
					body[0] = '!' // The retained snapshot must own its bytes.
				}
				if mode == "oversized" {
					e.CaptureOutbound([]byte(strings.Repeat("x", (4<<20)+1)))
					want = ""
				}
			}
			e.FinishRecord(req)
			got, err := svc.Sent(1, req.RequestID)
			if want == "" {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatal("unavailable outbound should not return an initial/stale body", err)
				}
			} else if err != nil || got != want {
				t.Fatalf("outbound snapshot: got %s, want %s, error %v", got, want, err)
			}
		})
	}
}

// History must remain readable and clearable even if an administrator needs to
// repair the policy. Exercise persisted history beyond the in-memory limit.
func TestRetainedHistoryDoesNotDependOnPolicy(t *testing.T) {
	svc := NewService(&memoryPrivacySettings{raw: "{invalid"})
	svc.SetDirectory(t.TempDir())
	dir := svc.runtime.recordsDir(scopeKey{1})
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	f, err := os.Create(filepath.Join(dir, now.Format("2006-01-02")+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 450; i++ {
		r := Record{ID: fmt.Sprint(i), Time: now.Add(time.Duration(i) * time.Millisecond), ProviderID: 7, Values: []Value{{Kind: "TERM", Action: "mask", By: "word", Count: 2}}}
		if err := json.NewEncoder(f).Encode(r); err != nil {
			f.Close()
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	page, err := svc.Records(1, RecordsQuery{Page: 3, Limit: 200, Kind: "TERM", ProviderID: 7})
	if err != nil || page.Total != 450 || len(page.Items) != 50 || page.Items[49].ID != "0" {
		t.Fatalf("retained page: %+v %v", page, err)
	}
	record, err := svc.Record(1, "0")
	if err != nil || record.ID != "0" {
		t.Fatal("oldest retained record unavailable", err)
	}
	summary, err := svc.Summary(1)
	if err != nil || summary.Requests != 450 || summary.Hits != 900 || summary.Masked != 900 || summary.Kinds["TERM"] != 900 || summary.Finders["word"] != 900 || summary.Providers[7] != 900 {
		t.Fatalf("incomplete retained summary: %+v %v", summary, err)
	}
	other, err := svc.Records(2, RecordsQuery{})
	if err != nil || other.Total != 0 {
		t.Fatal("history crossed tenants", err)
	}
	if err := svc.ClearRecords(1); err != nil {
		t.Fatal("cannot clear history while repairing policy", err)
	}
	if _, err := svc.Record(1, "0"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cleared record remains", err)
	}
}

func TestHistoryPaginationIsStableForEqualTimestamps(t *testing.T) {
	svc := NewService(&memoryPrivacySettings{})
	l := svc.history(1)
	now := time.Now()
	for i := 0; i < 100; i++ {
		l.recent = append(l.recent, keptRecord{record: Record{ID: fmt.Sprintf("%03d", i), Time: now, Values: []Value{{Kind: "TERM", Count: 1}}}})
	}
	seen := map[string]bool{}
	for page := 1; page <= 5; page++ {
		out, err := svc.Records(1, RecordsQuery{Page: page, Limit: 20})
		if err != nil || out.Total != 100 || len(out.Items) != 20 {
			t.Fatalf("page %d: %+v %v", page, out, err)
		}
		for _, record := range out.Items {
			if seen[record.ID] {
				t.Fatalf("same-time records overlap pages: %s", record.ID)
			}
			seen[record.ID] = true
			record.Values[0].Kind = "modified response"
		}
	}
	summary, err := svc.Summary(1)
	if err != nil || summary.Kinds["TERM"] != 100 || summary.Kinds["modified response"] != 0 {
		t.Fatal("query mutated retained records", err)
	}
}
