package privacy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

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
