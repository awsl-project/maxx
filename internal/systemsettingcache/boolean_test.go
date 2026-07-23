package systemsettingcache

import (
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type stubRepo struct {
	values []string
	errs   []error
	reads  int
}

func (r *stubRepo) Get(key string) (string, error) {
	r.reads++
	idx := r.reads - 1
	if idx < len(r.errs) && r.errs[idx] != nil {
		return "", r.errs[idx]
	}
	if idx < len(r.values) {
		return r.values[idx], nil
	}
	if len(r.values) > 0 {
		return r.values[len(r.values)-1], nil
	}
	return "", nil
}

func (r *stubRepo) Set(key, value string) error              { return nil }
func (r *stubRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r *stubRepo) Delete(key string) error                  { return nil }

func TestGetBooleanCachesFreshValue(t *testing.T) {
	oldTTL := BooleanTTL
	BooleanTTL = time.Hour
	defer func() { BooleanTTL = oldTTL }()

	key := "proxy_requests_disabled"
	Invalidate(key)

	repo := &stubRepo{values: []string{"true"}}
	if !GetBoolean(repo, key) {
		t.Fatal("expected first read to return true")
	}
	if !GetBoolean(repo, key) {
		t.Fatal("expected cached read to return true")
	}
	if repo.reads != 1 {
		t.Fatalf("reads = %d, want 1", repo.reads)
	}
}

func TestGetBooleanFallsBackToLastKnownValueOnRefreshError(t *testing.T) {
	oldTTL := BooleanTTL
	BooleanTTL = time.Nanosecond
	defer func() { BooleanTTL = oldTTL }()

	key := "proxy_requests_disabled"
	Invalidate(key)

	repo := &stubRepo{
		values: []string{"true"},
		errs:   []error{nil, errors.New("db temporarily unavailable")},
	}
	if !GetBoolean(repo, key) {
		t.Fatal("expected first read to return true")
	}
	time.Sleep(time.Millisecond)
	if !GetBoolean(repo, key) {
		t.Fatal("expected stale cached true value on refresh error")
	}
	if repo.reads != 2 {
		t.Fatalf("reads = %d, want 2", repo.reads)
	}
}
