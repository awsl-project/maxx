package pii

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// waitFor waits for ok, up to 10s.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: never", what)
		}
	}
}

// within10s runs f, and fails the test when it hasn't returned in 10s: what
// it waits on is a request stuck for an hour.
func within10s[T any](t *testing.T, what string, f func() T) T {
	t.Helper()
	got := make(chan T, 1)
	go func() { got <- f() }()
	select {
	case v := <-got:
		return v
	case <-time.After(10 * time.Second):
		t.Fatalf("%s waited for the request pf-serve is stuck on", what)
		panic("unreachable")
	}
}

type found struct {
	fs  [][]redact.Found
	err error
}

func findAsync(ctx context.Context, texts ...string) chan found {
	c := make(chan found, 1)
	go func() {
		fs, err := FindContext(ctx, texts)
		c <- found{fs, err}
	}()
	return c
}

// #3682: a request pf-serve took long over held the lock the status and
// Stop take, and the Settings page and turning the model off waited with
// every request behind it. Now neither waits: the status says it runs,
// and Stop ends it, the request on it failing.
func TestStatusAndStopDontWaitForARequest(t *testing.T) {
	log := fakeInstall(t, "hang", false)
	was := classifyTimeout
	classifyTimeout = func(int) time.Duration { return time.Hour }
	t.Cleanup(func() { classifyTimeout = was })
	stuck := findAsync(context.Background(), "hi Alice")
	waitFor(t, "the request reaching pf-serve", func() bool { return len(fakeLines(t, log, "request")) == 1 })
	if st := within10s(t, "Now", Now); !st.Running || st.Loading {
		t.Fatalf("Now = %+v, want running", st)
	}
	within10s(t, "Stop", func() bool { Stop(); return true })
	r := within10s(t, "the stopped request", func() found { return <-stuck })
	if !errors.Is(r.err, errStopped) {
		t.Fatalf("the request on a stopped pf-serve: %v, want errStopped", r.err)
	}
	if n := len(fakeLines(t, log, "start")); n != 1 {
		t.Fatalf("started %d times, want once: a stopped one isn't started again", n)
	}
}

// The same while pf-serve loads the model (up to loadTimeout, 3m): the
// status says it is loading, and Stop ends the load.
func TestStatusAndStopDontWaitForALoad(t *testing.T) {
	log := fakeInstall(t, "loadhang", false)
	loading := findAsync(context.Background(), "hi Alice")
	waitFor(t, "pf-serve starting", func() bool { return len(fakeLines(t, log, "start")) == 1 })
	if st := within10s(t, "Now", Now); !st.Loading || st.Running {
		t.Fatalf("Now = %+v, want loading", st)
	}
	within10s(t, "Stop", func() bool { Stop(); return true })
	if r := within10s(t, "the request", func() found { return <-loading }); !errors.Is(r.err, errStopped) {
		t.Fatalf("the request whose load was stopped: %v", r.err)
	}
}

// A request waiting for its turn leaves when its context ends (its client
// went away, or the gateway answered it 504), not when the one ahead of
// it is done; one on pf-serve too, while its turn is finished all the
// same.
func TestWaiterLeavesWhenItsContextEnds(t *testing.T) {
	log := fakeInstall(t, "hang", false)
	was := classifyTimeout
	classifyTimeout = func(int) time.Duration { return time.Hour }
	t.Cleanup(func() { classifyTimeout = was })
	ahead, cancelAhead := context.WithCancel(context.Background())
	first := findAsync(ahead, "hi Alice")
	waitFor(t, "the first request reaching pf-serve", func() bool { return len(fakeLines(t, log, "request")) == 1 })

	ctx, cancel := context.WithCancelCause(context.Background())
	second := findAsync(ctx, "another Alice")
	waitFor(t, "the second request waiting", func() bool { return Now().Waiting == 1 })
	gaveUp := errors.New("gave up")
	cancel(gaveUp)
	r := within10s(t, "the request whose context ended", func() found { return <-second })
	if !errors.Is(r.err, gaveUp) || !strings.Contains(r.err.Error(), "waiting for the local masking model") {
		t.Fatalf("the waiter: %v", r.err)
	}
	if n := Now().Waiting; n != 0 {
		t.Fatalf("Waiting = %d after the waiter left", n)
	}
	cancelAhead()
	if r := within10s(t, "the request on pf-serve", func() found { return <-first }); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("the request on pf-serve: %v", r.err)
	}
	if n := len(fakeLines(t, log, "request")); n != 1 {
		t.Fatalf("pf-serve was asked %d times, want once: the waiter never got its turn", n)
	}
}

// Requests queued behind one another after a restart had most of their
// history in common: each was asked about all over again when its turn
// came. Now one reads the cache again when its turn comes.
func TestQueuedRequestReadsWhatTheOneAheadLearned(t *testing.T) {
	log := fakeInstall(t, "slow", false)
	history := []string{"hi Alice", "nothing here", "Bob and Alice"}
	first := findAsync(context.Background(), history...)
	waitFor(t, "the first request reaching pf-serve", func() bool { return len(fakeLines(t, log, "request")) == 1 })
	second := findAsync(context.Background(), history...)
	a, b := <-first, <-second
	if a.err != nil || b.err != nil || !reflect.DeepEqual(a.fs, b.fs) {
		t.Fatalf("answers: %v %v / %v %v", a.fs, a.err, b.fs, b.err)
	}
	if reqs := fakeLines(t, log, "request"); len(reqs) != 1 {
		t.Fatalf("pf-serve was asked %d times, want once: %q", len(reqs), reqs)
	}
}

// restart is magpie started again: nothing in memory, pf-serve not running.
func restart(t *testing.T) {
	t.Helper()
	Stop()
	cache.mu.Lock()
	cache.ll.Init()
	clear(cache.m)
	cache.mu.Unlock()
	disk.mu.Lock()
	disk.loaded = ""
	disk.mu.Unlock()
}

// #3682: after a restart every request's whole history went to the model
// again, one request after another, minutes of it on a CPU. What was found
// is kept on disk, by hash, without the text, and read back.
func TestRestartReadsWhatWasFound(t *testing.T) {
	log := fakeInstall(t, "ok", false)
	saveAfter = 10 * time.Millisecond
	t.Cleanup(func() { saveAfter = 2 * time.Second })
	history := []string{"hi Alice", "nothing here", "Bob, Alice and Alice"}
	want, err := Find(history)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the cache file", func() bool { _, err := os.Stat(cachePath()); return err == nil })
	b, _ := os.ReadFile(cachePath())
	for _, text := range history {
		if strings.Contains(string(b), text) {
			t.Fatalf("the cache file holds the text %q", text)
		}
	}
	restart(t)
	got, err := Find(append(history, "new Alice"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[:3], want) || !reflect.DeepEqual(got[3], []redact.Found{person(4, 9)}) {
		t.Fatalf("after a restart: %v, want %v and the new text's", got, want)
	}
	reqs := fakeLines(t, log, "request")
	if len(reqs) != 2 || strings.Contains(reqs[1], "Bob") || !strings.Contains(reqs[1], "new Alice") {
		t.Fatalf("requests = %q, want the one after the restart to ask about the new text alone", reqs)
	}
}

// A cache file cut short, or not one, is read as empty: the model is asked
// again, and its answers are right.
func TestBadCacheFileIsReadAsEmpty(t *testing.T) {
	log := fakeInstall(t, "ok", false)
	if _, err := Find([]string{"hi Alice", "Bob, Alice"}); err != nil {
		t.Fatal(err)
	}
	if err := saveCache(); err != nil {
		t.Fatal(err)
	}
	whole, _ := os.ReadFile(cachePath())
	for i, bad := range [][]byte{whole[:len(whole)-3], []byte("not a cache"), append(append([]byte{}, whole...), 0)} {
		restart(t)
		if err := os.WriteFile(cachePath(), bad, 0o644); err != nil {
			t.Fatal(err)
		}
		before := len(fakeLines(t, log, "request"))
		got, err := Find([]string{"hi Alice"})
		if err != nil || !reflect.DeepEqual(got, [][]redact.Found{{person(3, 8)}}) {
			t.Fatalf("file %d: %v %v", i, got, err)
		}
		if len(fakeLines(t, log, "request")) != before+1 {
			t.Fatalf("file %d: a bad file's answers were used", i)
		}
	}
}

// A file written before a span kept its score (cacheMagic1) is read, its
// spans as sure: masked at any threshold, as they were before, and not
// asked about again.
func TestOldCacheFileIsReadAsSure(t *testing.T) {
	log := fakeInstall(t, "ok", false)
	var b bytes.Buffer
	b.WriteString(cacheMagic1)
	k := sumOf("hi Alice")
	binary.Write(&b, binary.LittleEndian, uint32(1))
	b.Write(k[:])
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, [2]uint32{3, 8})
	b.WriteByte(byte(len("private_person")))
	b.WriteString("private_person")
	if err := os.WriteFile(cachePath(), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	restart(t)
	got, err := Find([]string{"hi Alice"})
	if err != nil || !reflect.DeepEqual(got, [][]redact.Found{{{Start: 3, End: 8, Label: "private_person", Score: 1}}}) {
		t.Fatalf("Find = %v, %v", got, err)
	}
	if reqs := fakeLines(t, log, "request"); len(reqs) != 0 {
		t.Fatalf("asked again: %q", reqs)
	}
}

// A span read back that doesn't fit its text is asked about again.
func TestCachedSpanOutsideItsTextIsAskedAgain(t *testing.T) {
	log := fakeInstall(t, "ok", false)
	cache.mu.Lock()
	cache.put(sumOf("hi Alice"), []redact.Found{person(3, 80)})
	cache.mu.Unlock()
	disk.mu.Lock()
	disk.loaded = cachePath()
	disk.mu.Unlock()
	got, err := Find([]string{"hi Alice"})
	if err != nil || !reflect.DeepEqual(got, [][]redact.Found{{person(3, 8)}}) {
		t.Fatalf("Find = %v %v", got, err)
	}
	if n := len(fakeLines(t, log, "request")); n != 1 {
		t.Fatalf("asked %d times, want once", n)
	}
}

func sumOf(s string) [32]byte { return sha256.Sum256([]byte(s)) }

// A request that stopped waiting leaves the chunks after the one the
// model is on: the model isn't kept reading for no one, holding the turn
// every later request (each waiting a short while on by default) needs.
// The old code took the turn again on a select that could pick either.
func TestStoppedRequestLeavesTheRestOfItsChunks(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	first, release := make(chan struct{}), make(chan struct{})
	b := &backend{turn: make(chan struct{}, 1), waiting: new(atomic.Int32), cache: newLRU(100), disk: new(diskState),
		path:     func() string { return filepath.Join(dir, "found.bin") },
		heldBack: func() error { return nil },
		classify: func(texts []string) ([][]redact.Found, error) {
			if calls.Add(1) == 1 {
				close(first)
				<-release
			}
			return make([][]redact.Found, len(texts)), nil
		}}
	t.Cleanup(func() {
		b.disk.mu.Lock()
		if b.disk.timer != nil && b.disk.timer.Stop() {
			b.disk.saves.Done() // the write that won't run now
		}
		b.disk.timer = nil
		b.disk.mu.Unlock()
		b.disk.saves.Wait()
	})
	// three texts of a chunk each
	texts := []string{strings.Repeat("a ", chunkSize/2-8), strings.Repeat("b ", chunkSize/2-8), strings.Repeat("c ", chunkSize/2-8)}
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() { _, err := b.find(ctx, texts); got <- err }()
	<-first
	cancel()
	if err := <-got; err == nil {
		t.Fatal("a stopped request answered")
	}
	close(release)
	// the chunk under way is finished and its turn given back. Nothing
	// here waits to send on the turn: a waiting sender would be handed
	// it first, and the old code's select would never see it free
	waitFor(t, "the turn given back", func() bool { return len(b.turn) == 0 && calls.Load() >= 1 })
	// a chunk read after it would be read at once: watch for one a while
	for range 40 {
		if n := calls.Load(); n != 1 {
			t.Fatalf("%d chunks read for a request that stopped after the first", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	waitFor(t, "the turn free", func() bool { return len(b.turn) == 0 })
}
