package privacy

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// Records deliberately contain no message text, originals, session IDs, or
// provider credentials. Only bounded recent originals/sent bodies live in RAM.
type Value struct {
	Placeholder string `json:"placeholder"`
	Kind        string `json:"kind"`
	By          string `json:"by"`
	Hint        string `json:"hint,omitempty"`
	Length      int    `json:"length"`
	Count       int    `json:"count"`
	Action      string `json:"action"`
	Restored    int    `json:"restored"`
}
type Record struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	ProviderID uint64    `json:"providerId"`
	RequestID  uint64    `json:"requestId"`
	Status     string    `json:"status"`
	Bypassed   bool      `json:"bypassed"`
	TookUS     int64     `json:"tookUs"`
	Values     []Value   `json:"values"`
}
type RecordsQuery struct {
	Page, Limit      int
	Kind, Action, By string
	ProviderID       uint64
	Since, Until     time.Time
}
type RecordsPage struct {
	Items []Record `json:"items"`
	Total int      `json:"total"`
	Page  int      `json:"page"`
	Limit int      `json:"limit"`
}
type Summary struct {
	Requests, Hits, Restored, Masked, Warned, Blocked, Bypassed int
	Kinds                                                       map[string]int
	Finders                                                     map[string]int
	Providers                                                   map[uint64]int
}
type keptRecord struct {
	record    Record
	originals map[string]string
	sent      string
	size      int
}
type ledger struct {
	mu     sync.Mutex
	recent []keptRecord
	bytes  int
}

func (s *projectScope) history() *ledger { return &s.records }
func (e *Exchange) addRestored(counts map[string]int) {
	if e == nil || len(counts) == 0 {
		return
	}
	if e.restored == nil {
		e.restored = map[string]int{}
	}
	for p, n := range counts {
		e.restored[p] += n
	}
}
func (r *Runtime) recordsDir(key scopeKey) string {
	if r.directory == "" {
		return ""
	}
	return filepath.Join(r.directory, strconv.FormatUint(key.tenant, 10), strconv.FormatUint(key.project, 10), "records")
}
func (e *Exchange) BeginRecord(req *domain.ProxyRequest) {
	if e == nil || req == nil || e.runtime == nil {
		return
	}
	e.recordID = req.RequestID
	e.recordTime = req.StartTime
	if e.recordTime.IsZero() {
		e.recordTime = time.Now()
	}
	e.runtime.recordMu.Lock()
	e.recordGeneration = e.runtime.generations[e.key]
	e.runtime.recordMu.Unlock()
}
func (e *Exchange) FinishRecord(req *domain.ProxyRequest) {
	if e == nil || req == nil || e.recorded || len(e.Hits) == 0 && !e.Bypassed {
		return
	}
	e.recorded = true
	if e.recordID == "" {
		e.BeginRecord(req)
	}
	record := Record{ID: e.recordID, Time: e.recordTime, RequestID: req.ID, ProviderID: req.ProviderID, Status: req.Status, Bypassed: e.Bypassed, TookUS: e.tookUS, Values: []Value{}}
	indexes := map[string]int{}
	for _, h := range e.Hits {
		key := h.Placeholder + "|" + h.Kind + "|" + h.Action
		if at, ok := indexes[key]; ok {
			record.Values[at].Count++
			continue
		}
		indexes[key] = len(record.Values)
		action := h.Action
		if action == "" {
			action = "mask"
		}
		record.Values = append(record.Values, Value{Placeholder: h.Placeholder, Kind: h.Kind, By: h.By, Hint: strings.Clone(h.Head), Length: h.Len, Count: 1, Action: action, Restored: e.restored[h.Placeholder]})
	}
	originals := make(map[string]string, len(e.pairs))
	size := len(e.Body)
	for p, v := range e.pairs {
		originals[p] = v
		size += len(p) + len(v)
	}
	sent := string(e.Body)
	if size > 4<<20 {
		originals = nil
		sent = ""
		size = 0
	}
	l := e.runtime.scope(e.key).history()
	l.mu.Lock()
	defer l.mu.Unlock()
	e.runtime.recordMu.Lock()
	defer e.runtime.recordMu.Unlock()
	// Clear also fences requests whose learned scope was evicted.
	if e.recordGeneration != e.runtime.generations[e.key] {
		return
	}
	l.recent = append(l.recent, keptRecord{record: record, originals: originals, sent: sent, size: size})
	l.bytes += size
	for len(l.recent) > 400 || l.bytes > 16<<20 {
		l.bytes -= l.recent[0].size
		l.recent = l.recent[1:]
	}
	directory := e.runtime.recordsDir(e.key)
	if directory == "" {
		return
	}
	if os.MkdirAll(directory, 0700) != nil {
		return
	}
	pruneRecords(directory, time.Now())
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(directory, record.Time.UTC().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		_, _ = f.Write(append(data, '\n'))
		_ = f.Close()
	}
}
func pruneRecords(directory string, now time.Time) {
	files, _ := os.ReadDir(directory)
	total := int64(0)
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}
		day, err := time.Parse("2006-01-02", strings.TrimSuffix(file.Name(), ".jsonl"))
		if err != nil {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		if now.UTC().Sub(day) > 30*24*time.Hour || total > 100<<20 {
			_ = os.Remove(filepath.Join(directory, file.Name()))
		}
	}
}
func (s *Service) authorizedHistory(tenant, project uint64) (*ledger, error) {
	if _, err := s.Configuration(tenant, project); err != nil {
		return nil, err
	}
	return s.runtime.scope(scopeKey{tenant, project}).history(), nil
}
func (s *Service) Records(tenant, project uint64, q RecordsQuery) (RecordsPage, error) {
	l, err := s.authorizedHistory(tenant, project)
	if err != nil {
		return RecordsPage{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	records := map[string]Record{}
	directory := s.runtime.recordsDir(scopeKey{tenant, project})
	if directory != "" {
		pruneRecords(directory, time.Now())
		files, _ := filepath.Glob(filepath.Join(directory, "*.jsonl"))
		for _, path := range files {
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 64<<10), 2<<20)
			for scanner.Scan() {
				var record Record
				if json.Unmarshal(scanner.Bytes(), &record) == nil {
					records[record.ID] = record
				}
			}
			f.Close()
		}
	}
	for _, kept := range l.recent {
		records[kept.record.ID] = kept.record
	}
	all := make([]Record, 0, len(records))
	for _, record := range records {
		if q.ProviderID != 0 && q.ProviderID != record.ProviderID || !q.Since.IsZero() && record.Time.Before(q.Since) || !q.Until.IsZero() && record.Time.After(q.Until) {
			continue
		}
		if q.Kind != "" || q.Action != "" || q.By != "" {
			matched := false
			for _, v := range record.Values {
				if (q.Kind == "" || v.Kind == q.Kind) && (q.Action == "" || v.Action == q.Action) && (q.By == "" || v.By == q.By) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		record.Values = append([]Value(nil), record.Values...)
		all = append(all, record)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Time.After(all[j].Time) })
	page, limit := q.Page, q.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	out := RecordsPage{Items: []Record{}, Total: len(all), Page: page, Limit: limit}
	start := (page - 1) * limit
	if start < 0 || start >= len(all) {
		return out, nil
	}
	end := min(start+limit, len(all))
	out.Items = all[start:end]
	return out, nil
}
func (s *Service) Record(tenant, project uint64, id string) (Record, error) {
	// Lists are bounded, but a detail lookup must cover the retained history.
	for page := 1; ; page++ {
		out, err := s.Records(tenant, project, RecordsQuery{Page: page, Limit: 200})
		if err != nil {
			return Record{}, err
		}
		for _, record := range out.Items {
			if record.ID == id {
				return record, nil
			}
		}
		if page*200 >= out.Total {
			return Record{}, domain.ErrNotFound
		}
	}
}
func (s *Service) Summary(tenant, project uint64) (Summary, error) {
	out := Summary{Kinds: map[string]int{}, Finders: map[string]int{}, Providers: map[uint64]int{}}
	for page := 1; ; page++ {
		result, err := s.Records(tenant, project, RecordsQuery{Page: page, Limit: 200})
		if err != nil {
			return out, err
		}
		for _, record := range result.Items {
			out.Requests++
			if record.Bypassed {
				out.Bypassed++
			}
			for _, v := range record.Values {
				out.Hits += v.Count
				out.Restored += v.Restored
				out.Kinds[v.Kind] += v.Count
				out.Finders[v.By] += v.Count
				out.Providers[record.ProviderID] += v.Count
				switch v.Action {
				case "warn":
					out.Warned += v.Count
				case "block":
					out.Blocked += v.Count
				default:
					out.Masked += v.Count
				}
			}
		}
		if page*200 >= result.Total {
			return out, nil
		}
	}
}
func (s *Service) Original(tenant, project uint64, id, placeholder string) (string, error) {
	l, err := s.authorizedHistory(tenant, project)
	if err != nil {
		return "", err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, kept := range l.recent {
		if kept.record.ID == id {
			value, ok := kept.originals[placeholder]
			if ok {
				return value, nil
			}
		}
	}
	return "", domain.ErrNotFound
}
func (s *Service) Sent(tenant, project uint64, id string) (string, error) {
	l, err := s.authorizedHistory(tenant, project)
	if err != nil {
		return "", err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, kept := range l.recent {
		if kept.record.ID == id && kept.sent != "" {
			return kept.sent, nil
		}
	}
	return "", domain.ErrNotFound
}
func (s *Service) ClearRecords(tenant, project uint64) error {
	l, err := s.authorizedHistory(tenant, project)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s.runtime.recordMu.Lock()
	defer s.runtime.recordMu.Unlock()
	directory := s.runtime.recordsDir(scopeKey{tenant, project})
	if directory != "" {
		if err := os.RemoveAll(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	l.recent = nil
	l.bytes = 0
	s.runtime.generations[scopeKey{tenant, project}]++
	s.runtime.scope(scopeKey{tenant, project}).state.Clear()
	return nil
}
