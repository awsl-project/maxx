package pii

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/privacy/pii/guard"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// guardSandbox gives AstrLink Guard a folder of the test's own, and
// puts back what the test changed.
func guardSandbox(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	wasRoot, wasFiles, wasBase, wasLoad, wasIdle := guardRoot, guardFiles, guardBase, guardLoad, idleAfter
	wasDir := dir
	guardRoot = func() string { return filepath.Join(d, "astrlink-guard") }
	dir = func() string { return filepath.Join(d, "privacy-filter") }
	t.Cleanup(func() {
		guardModel{}.Cancel()
		guardModel{}.Stop()
		for deadline := time.Now().Add(30 * time.Second); (guardModel{}).Now().Downloading && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		guardBackend.disk.mu.Lock()
		if guardBackend.disk.timer != nil && guardBackend.disk.timer.Stop() {
			guardBackend.disk.saves.Done()
		}
		guardBackend.disk.timer, guardBackend.disk.loaded = nil, ""
		guardBackend.disk.mu.Unlock()
		guardBackend.disk.saves.Wait()
		guardBackend.cache.mu.Lock()
		guardBackend.cache.ll.Init()
		clear(guardBackend.cache.m)
		guardBackend.cache.mu.Unlock()
		gsrv.mu.Lock()
		gsrv.failed, gsrv.loadMS = "", 0
		gsrv.mu.Unlock()
		guardRoot, guardFiles, guardBase, guardLoad, idleAfter = wasRoot, wasFiles, wasBase, wasLoad, wasIdle
		dir = wasDir
	})
	return d
}

// fakeGuardFiles puts files of the pinned sizes where the model goes.
func fakeGuardFiles(t *testing.T) {
	t.Helper()
	guardFiles = []pinned{{"int8/model_int8.onnx", 3, "x"}, {"int8/model_int8.onnx_data", 5, "x"}, {"int8/tokenizer.json", 2, "x"}}
	os.MkdirAll(guardDir(), 0o755)
	for _, f := range guardFiles {
		if err := os.WriteFile(filepath.Join(guardDir(), filepath.Base(f.path)), make([]byte, f.size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeFinder finds each word of its map in a text, by the model's own
// label, and counts the texts it read.
type fakeFinder struct {
	words map[string]string
	reads *atomic.Int32
}

func (f fakeFinder) Find(text string) []guard.Span {
	f.reads.Add(1)
	var out []guard.Span
	for w, l := range f.words {
		if i := strings.Index(text, w); i >= 0 {
			out = append(out, guard.Span{Start: i, End: i + len(w), Label: l, Score: 0.99})
		}
	}
	return out
}

func TestChoose(t *testing.T) {
	guardSandbox(t)
	for _, c := range []struct {
		name string
		on   bool
		want string
	}{
		{"", false, AstrLinkGuard},            // a new user
		{"", true, AstrLinkGuard},             // on, Privacy Filter not downloaded
		{AstrLinkGuard, true, AstrLinkGuard},  // picked
		{PrivacyFilter, false, PrivacyFilter}, // picked
		{"something-else", false, AstrLinkGuard},
	} {
		if got := Choose(c.name, c.on); got != c.want {
			t.Errorf("Choose(%q, %v) = %q, want %q", c.name, c.on, got, c.want)
		}
	}
	// Privacy Filter downloaded: turned off, AstrLink Guard is the
	// default; on, it masks until AstrLink Guard is downloaded
	os.MkdirAll(dir(), 0o755)
	os.WriteFile(modelPath(), make([]byte, 1), 0o644)
	wasSize := modelSize
	modelSize = 1
	defer func() { modelSize = wasSize }()
	os.MkdirAll(serveDir(), 0o755)
	os.WriteFile(exe("pf-serve"), []byte("x"), 0o755)
	if got := Choose("", false); got != AstrLinkGuard {
		t.Errorf("with Privacy Filter on disk and off, Choose = %q", got)
	}
	if got := Choose("", true); got != PrivacyFilter {
		t.Errorf("with Privacy Filter on disk and on, Choose = %q", got)
	}
}

func TestOfAndModels(t *testing.T) {
	if Of(PrivacyFilter).ID() != PrivacyFilter || Of(AstrLinkGuard).ID() != AstrLinkGuard {
		t.Fatal("Of names the wrong model")
	}
	ms := Models()
	if len(ms) != 2 || ms[0].ID() != PrivacyFilter || ms[1].ID() != AstrLinkGuard {
		t.Fatalf("Models = %v", ms)
	}
	if !Of(AstrLinkGuard).Supported() {
		t.Fatal("AstrLink Guard runs in Go everywhere")
	}
	if got := Of(AstrLinkGuard).Size(); got != 330781+37124608+1502881 {
		t.Fatalf("size = %d", got)
	}
}

// What AstrLink Guard finds reaches redact under Privacy Filter's labels,
// which are the ones redact's Model masks; a text read once is not read
// again.
func TestGuardFindsUnderRedactsLabels(t *testing.T) {
	guardSandbox(t)
	fakeGuardFiles(t)
	var reads, loads atomic.Int32
	guardLoad = func(string) (spanFinder, error) {
		loads.Add(1)
		return fakeFinder{map[string]string{"13800138000": "phone", "张三": "private_person", "sk-abc": "common_secret", "6222020200112233445": "payment_card", "https://a.example": "url"}, &reads}, nil
	}
	texts := []string{"张三的电话是13800138000", "key sk-abc", "卡号 6222020200112233445 https://a.example", "nothing"}
	got, err := Of(AstrLinkGuard).Find(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	labels := func(fs []redact.Found) map[string]bool {
		m := map[string]bool{}
		for _, f := range fs {
			m[f.Label] = true
		}
		return m
	}
	if l := labels(got[0]); !l["private_phone"] || !l["private_person"] || len(l) != 2 {
		t.Errorf("text 0: %v", got[0])
	}
	if l := labels(got[1]); !l["secret"] || len(l) != 1 {
		t.Errorf("text 1: %v", got[1])
	}
	if l := labels(got[2]); !l["account_number"] || !l["private_url"] || len(l) != 2 {
		t.Errorf("text 2: %v", got[2])
	}
	if len(got[3]) != 0 {
		t.Errorf("text 3: %v", got[3])
	}
	if f := got[0][0]; texts[0][f.Start:f.End] != "张三" && texts[0][f.Start:f.End] != "13800138000" {
		t.Errorf("span %v is %q", f, texts[0][f.Start:f.End])
	}
	n := reads.Load()
	if _, err := Of(AstrLinkGuard).Find(context.Background(), texts); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != n || loads.Load() != 1 {
		t.Fatalf("asked again: %d reads, %d loads", reads.Load()-n, loads.Load())
	}
	st := Of(AstrLinkGuard).Now()
	if st.Model != AstrLinkGuard || !st.Installed || !st.Running || st.Device != "CPU" {
		t.Fatalf("Now = %+v", st)
	}
	if pst := Of(PrivacyFilter).Now(); pst.Model != PrivacyFilter || pst.Running {
		t.Fatalf("Privacy Filter's Now = %+v", pst)
	}
}

// One whose files aren't on disk fails, rather than mask by the rules
// alone; one that didn't load is held back at once until it is tried
// again.
func TestGuardNotInstalledAndHeldBack(t *testing.T) {
	guardSandbox(t)
	if _, err := Of(AstrLinkGuard).Find(context.Background(), []string{"a"}); err != ErrNotInstalled {
		t.Fatalf("err = %v", err)
	}
	fakeGuardFiles(t)
	var loads atomic.Int32
	guardLoad = func(string) (spanFinder, error) {
		loads.Add(1)
		return nil, os.ErrInvalid
	}
	if _, err := Of(AstrLinkGuard).Find(context.Background(), []string{"b"}); err == nil {
		t.Fatal("a model that didn't load answered")
	}
	_, err := Of(AstrLinkGuard).Find(context.Background(), []string{"c"})
	if err == nil || !strings.Contains(err.Error(), "tried again in") || loads.Load() != 1 {
		t.Fatalf("err = %v after %d loads", err, loads.Load())
	}
	if st := Of(AstrLinkGuard).Now(); st.Failed == "" {
		t.Fatalf("Now = %+v", st)
	}
	Of(AstrLinkGuard).Stop() // turned off and on: tried at once
	if _, err := Of(AstrLinkGuard).Find(context.Background(), []string{"d"}); err == nil || loads.Load() != 2 {
		t.Fatalf("err = %v after %d loads", err, loads.Load())
	}
}

// The model is let go after idleAfter with no request, and loaded again
// by the next.
func TestGuardLetGoWhenIdle(t *testing.T) {
	guardSandbox(t)
	fakeGuardFiles(t)
	idleAfter = 20 * time.Millisecond
	var reads, loads atomic.Int32
	guardLoad = func(string) (spanFinder, error) {
		loads.Add(1)
		return fakeFinder{nil, &reads}, nil
	}
	if _, err := Of(AstrLinkGuard).Find(context.Background(), []string{"one"}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); Of(AstrLinkGuard).Now().Running; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("still loaded")
		}
	}
	if _, err := Of(AstrLinkGuard).Find(context.Background(), []string{"two"}); err != nil || loads.Load() != 2 {
		t.Fatalf("err = %v, %d loads", err, loads.Load())
	}
}

// Download fetches each file, checked against its pin, from the
// publisher's address at the pinned revision; a file whose bytes don't
// match is not kept.
func TestGuardDownload(t *testing.T) {
	guardSandbox(t)
	body := map[string][]byte{"/r/aaaabbbbcccc1/int8/model_int8.onnx": []byte("graph"), "/r/aaaabbbbcccc1/int8/model_int8.onnx_data": []byte("weights!"), "/r/aaaabbbbcccc1/int8/tokenizer.json": []byte("{}")}
	f := &files{m: map[string][]byte{}}
	for k, v := range body {
		f.m[k] = v
	}
	s := httptest.NewServer(f)
	defer s.Close()
	wasRev := guardRev
	defer func() { guardRev = wasRev }()
	guardRev, guardBase = "aaaabbbbcccc1", s.URL+"/r/"
	guardFiles = nil
	for _, p := range []string{"int8/model_int8.onnx", "int8/model_int8.onnx_data", "int8/tokenizer.json"} {
		b := body["/r/aaaabbbbcccc1/"+p]
		guardFiles = append(guardFiles, pinned{p, int64(len(b)), sum(b)})
	}
	// a folder of a revision pinned before goes once this one is in
	os.MkdirAll(filepath.Join(guardRoot(), "old"), 0o755)
	wait := func() Status {
		t.Helper()
		done := make(chan struct{})
		if err := Of(AstrLinkGuard).Download(func() { close(done) }); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(5 * time.Millisecond) {
			select {
			case <-done:
				return Of(AstrLinkGuard).Now()
			default:
			}
			st := Of(AstrLinkGuard).Now()
			if !st.Downloading && st.Error != "" {
				return st
			}
			if time.Now().After(deadline) {
				t.Fatalf("not done: %+v", st)
			}
		}
	}
	// the server's data file isn't the one pinned
	good := body["/r/aaaabbbbcccc1/int8/model_int8.onnx_data"]
	f.mu.Lock()
	f.m["/r/aaaabbbbcccc1/int8/model_int8.onnx_data"] = []byte("weights?")
	f.mu.Unlock()
	if st := wait(); !strings.Contains(st.Error, "checksum") || st.Installed {
		t.Fatalf("a file that doesn't match its pin: %+v", st)
	}
	f.mu.Lock()
	f.m["/r/aaaabbbbcccc1/int8/model_int8.onnx_data"] = good
	f.mu.Unlock()
	st := wait()
	if st.Error != "" || !st.Installed || st.Done != st.Total || st.Total != int64(len("graph")+len("weights!")+len("{}")) {
		t.Fatalf("Now = %+v", st)
	}
	if b, _ := os.ReadFile(filepath.Join(guardDir(), "model_int8.onnx_data")); string(b) != "weights!" {
		t.Fatalf("data = %q", b)
	}
	if _, err := os.Stat(filepath.Join(guardRoot(), "old")); err == nil {
		t.Fatal("the older revision's folder was left")
	}
	if err := Of(AstrLinkGuard).Remove(); err != nil || Of(AstrLinkGuard).Installed() {
		t.Fatalf("Remove: %v", err)
	}
}

// TestGuardFilesAreThePublishers checks each pinned file against the
// int8 lines of the publisher's own SHA256SUMS at guardRev (kept in
// testdata): its path in the repo and its sha256. A path the repo
// doesn't have is a 404 on every user's download.
func TestGuardFilesAreThePublishers(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "astrlink-SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	sums := map[string]string{}
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if sha, path, ok := strings.Cut(l, "  "); ok {
			sums[path] = sha
		}
	}
	for _, f := range guardFiles {
		if sums[f.path] != f.sha {
			t.Errorf("%s: pinned %s, the publisher lists %q", f.path, f.sha, sums[f.path])
		}
	}
}
