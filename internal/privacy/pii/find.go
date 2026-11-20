package pii

import (
	"container/list"
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// Find is redact's Model: what the model finds in each of texts. A
// request's history comes back turn after turn, so what was found in a
// text is kept (by its hash, on disk too: cache.go) and the model is
// asked only about texts it hasn't seen. Those go to it joined into
// chunks of up to chunkSize: one pass over a few joined texts takes about
// what one over a single short text does (100 short messages: 683 ms one
// by one, 44 ms joined, on an M4 Pro's GPU). A text longer than
// pieceSize goes in overlapping pieces (pieces), each kept on its own, so
// a long text is read a piece at a time and what was read is kept even
// when the rest isn't (#3682).
func Find(texts []string) ([][]redact.Found, error) {
	return FindContext(context.Background(), texts)
}

// FindContext is Find for a caller that may stop waiting: when ctx ends
// (the client went away) it returns at once, whether its turn hadn't come
// or pf-serve is on it. The chunk pf-serve is on is finished all the same
// and its answers kept, so the request sent again finds them; the chunks
// after it are left. Between chunks the turn is given up and taken again,
// so a short request waiting for it goes before the rest of a long one.
func FindContext(ctx context.Context, texts []string) ([][]redact.Found, error) {
	return pf.find(ctx, texts)
}

// backend is one model's side of FindContext: its turn and the requests
// waiting for it, what it found (cache, on disk at path), and how it is
// asked (classify, with turn held) unless it failed a while ago
// (heldBack).
type backend struct {
	turn     chan struct{}
	waiting  *atomic.Int32
	cache    *lru
	disk     *diskState
	path     func() string
	classify func([]string) ([][]redact.Found, error)
	heldBack func() error
}

// pf is Privacy Filter's, run by pf-serve.
var pf = &backend{turn: turn, waiting: &waiting, cache: cache, disk: &disk, path: cachePath,
	classify: classify, heldBack: heldBack}

func (b *backend) find(ctx context.Context, texts []string) ([][]redact.Found, error) {
	b.loadCache()
	out, miss := b.known(texts, nil)
	if len(miss) == 0 {
		return out, nil
	}
	if ctx.Err() != nil {
		return nil, waitErr(ctx)
	}
	if err := b.heldBack(); err != nil {
		return nil, err
	}
	if !b.takeTurn(ctx) {
		return nil, waitErr(ctx)
	}
	type result struct {
		out [][]redact.Found
		err error
	}
	done := make(chan result, 1)
	go func() {
		held := true
		defer func() {
			if held {
				<-b.turn
			}
		}()
		// what this call's chunks found, by hash, whatever the cache
		// lets go of meanwhile
		got := map[[32]byte][]redact.Found{}
		// read again: the requests before this one in turn had most of a
		// history in common with it, and asked already
		out, miss := b.known(texts, got)
		for c := 0; len(miss) > 0; c++ {
			if c > 0 {
				// a caller that stopped waiting leaves the chunks after
				// this one: the turn isn't taken again for them (a
				// select on a free turn and an ended ctx picks either)
				if ctx.Err() != nil {
					done <- result{nil, waitErr(ctx)}
					return
				}
				// give way to a request waiting for the turn: it goes
				// before the next chunk
				<-b.turn
				held = false
				if !b.takeTurn(ctx) {
					done <- result{nil, waitErr(ctx)}
					return
				}
				held = true
			}
			// the one ahead may have failed: those queued behind it
			// don't each try again, minutes apiece
			if err := b.heldBack(); err != nil {
				done <- result{nil, err}
				return
			}
			chunks, parts := join(b.missing(texts, miss, got))
			if len(chunks) == 0 {
				done <- result{nil, fmt.Errorf("the local masking model has no text to read for %d texts", len(miss))}
				return
			}
			read, err := b.classify(chunks[:1])
			if err != nil {
				done <- result{nil, err}
				return
			}
			b.cache.mu.Lock()
			for _, p := range parts[0] {
				k := sha256.Sum256([]byte(p.s))
				got[k] = within(read[0], p.at, p.at+len(p.s))
				b.cache.put(k, got[k])
			}
			b.cache.mu.Unlock()
			b.saveSoon()
			// the texts still missing, with this chunk's answers and
			// what others' requests found meanwhile
			sub := make([]string, len(miss))
			for j, i := range miss {
				sub[j] = texts[i]
			}
			o, m := b.known(sub, got)
			was := miss
			miss = nil
			for j, i := range was {
				out[i] = o[j]
			}
			for _, j := range m {
				miss = append(miss, was[j])
			}
		}
		done <- result{out, nil}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-ctx.Done():
		return nil, waitErr(ctx)
	}
}

// takeTurn waits for the model's turn while ctx lives, counted as
// waiting; false when ctx ended first.
func (b *backend) takeTurn(ctx context.Context) bool {
	b.waiting.Add(1)
	defer b.waiting.Add(-1)
	select {
	case b.turn <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// waitErr is a request's that stopped waiting for the model.
func waitErr(ctx context.Context) error {
	return fmt.Errorf("stopped waiting for the local masking model: %w", context.Cause(ctx))
}

// known is what is known of texts, from got (this call's answers) and the
// cache, and which texts it isn't. A text kept whole is taken as it is;
// a long one is put together from its pieces, once all of them are known,
// and kept whole then too.
func (b *backend) known(texts []string, got map[[32]byte][]redact.Found) (out [][]redact.Found, miss []int) {
	out = make([][]redact.Found, len(texts))
	b.cache.mu.Lock()
	defer b.cache.mu.Unlock()
	look := func(s string) ([]redact.Found, bool) {
		k := sha256.Sum256([]byte(s))
		if fs, ok := got[k]; ok {
			return fs, true
		}
		if fs, ok := b.cache.get(k); ok && fits(fs, s) {
			return fs, true
		}
		return nil, false
	}
	for i, t := range texts {
		if fs, ok := look(t); ok {
			out[i] = fs
			continue
		}
		ps := pieces(t)
		if len(ps) < 2 {
			miss = append(miss, i)
			continue
		}
		var all []redact.Found
		ok := true
		for _, p := range ps {
			fs, have := look(t[p.at:p.end])
			if !have {
				ok = false
				break
			}
			for _, f := range fs {
				all = append(all, redact.Found{Start: f.Start + p.at, End: f.End + p.at, Label: f.Label, Score: f.Score})
			}
		}
		if !ok {
			miss = append(miss, i)
			continue
		}
		out[i] = merge(all)
		b.cache.put(sha256.Sum256([]byte(t)), out[i])
	}
	return out, miss
}

// fits says every span of fs is inside t: one read from a damaged file
// that isn't is asked about again.
func fits(fs []redact.Found, t string) bool {
	for _, f := range fs {
		if f.Start < 0 || f.End > len(t) || f.Start >= f.End {
			return false
		}
	}
	return true
}

// chunkSize is how long a chunk of joined texts grows.
const chunkSize = 12 << 10

// pieceSize is how long a piece of a long text is at most, and
// pieceOverlap how much of the piece before it each piece after the first
// reads again, so a name or a secret cut by one piece's end is whole in
// the next. 12 KB is 1.6s of an RX 470/580's time, 0.4s of an M4 Pro's,
// well inside classifyTimeout; a 480 KB text whole was 90s on the RX
// 470/580, past the gateway's 2 minutes on its CPU, and kept nothing when
// cut off (#3682).
const (
	pieceSize    = chunkSize
	pieceOverlap = 1 << 10
)

// span is a piece of a text: t[at:end].
type span struct{ at, end int }

// pieces cuts t into pieces of at most pieceSize, each after the first
// beginning pieceOverlap before the one before ends. A piece ends after a
// line's end in its second half, else after a space there, else at a
// rune's start; the next begins after a line's end in the first half of
// the overlap, else after a space there, else at a rune's start. A text of pieceSize
// or less is one piece; an empty one none.
func pieces(t string) []span {
	if t == "" {
		return nil
	}
	if len(t) <= pieceSize {
		return []span{{0, len(t)}}
	}
	var out []span
	for at := 0; ; {
		end := at + pieceSize
		if end >= len(t) {
			return append(out, span{at, len(t)})
		}
		half := at + pieceSize/2
		if i := strings.LastIndexByte(t[half:end], '\n'); i >= 0 {
			end = half + i + 1
		} else if i := strings.LastIndexByte(t[half:end], ' '); i >= 0 {
			end = half + i + 1
		} else {
			for end > half && !utf8.RuneStart(t[end]) {
				end--
			}
		}
		out = append(out, span{at, end})
		next := end - pieceOverlap
		if i := strings.IndexByte(t[next:end-pieceOverlap/2], '\n'); i >= 0 {
			next += i + 1
		} else if i := strings.IndexByte(t[next:end-pieceOverlap/2], ' '); i >= 0 {
			next += i + 1
		} else {
			for !utf8.RuneStart(t[next]) {
				next++
			}
		}
		at = next
	}
}

// merge puts a long text's spans from its pieces together: spans that
// overlap (one found in two pieces, or cut by a piece's end in one and
// whole in the next) become one, its label the longer's, and spans of one
// label that touch become one. Sorted, with none twice.
func merge(fs []redact.Found) []redact.Found {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].Start != fs[j].Start {
			return fs[i].Start < fs[j].Start
		}
		return fs[i].End > fs[j].End
	})
	var out []redact.Found
	for _, f := range fs {
		if n := len(out); n > 0 {
			last := &out[n-1]
			if f.Start < last.End || f.Start == last.End && f.Label == last.Label {
				if f.End-f.Start > last.End-last.Start {
					last.Label = f.Label
				}
				last.End = max(last.End, f.End)
				// as sure as the surer: what either would mask is masked
				last.Score = max(last.Score, f.Score)
				continue
			}
		}
		out = append(out, f)
	}
	return out
}

// missing are what of texts[miss] neither got nor the cache has, to ask
// the model about: the texts whole, and the long ones' pieces, each once.
func (b *backend) missing(texts []string, miss []int, got map[[32]byte][]redact.Found) []string {
	var out []string
	seen := map[[32]byte]bool{}
	b.cache.mu.Lock()
	defer b.cache.mu.Unlock()
	for _, i := range miss {
		t := texts[i]
		for _, p := range pieces(t) {
			s := t[p.at:p.end]
			k := sha256.Sum256([]byte(s))
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, ok := got[k]; ok {
				continue
			}
			if fs, ok := b.cache.get(k); ok && fits(fs, s) {
				continue
			}
			out = append(out, s)
		}
		if t == "" {
			// as before: an empty text is asked about, and kept
			k := sha256.Sum256(nil)
			if !seen[k] {
				seen[k] = true
				out = append(out, "")
			}
		}
	}
	return out
}

// sep is put between joined texts: a paragraph's end, so the model reads
// each as a text of its own.
const sep = "\n\n"

// part is a text or piece in a chunk: s, at at.
type part struct {
	s  string
	at int
}

// join puts texts into chunks, and says where in which each is.
func join(texts []string) (chunks []string, parts [][]part) {
	cur, curParts := "", []part(nil)
	flush := func() {
		if curParts != nil {
			chunks, parts = append(chunks, cur), append(parts, curParts)
			cur, curParts = "", nil
		}
	}
	for _, t := range texts {
		if curParts != nil && len(cur)+len(sep)+len(t) > chunkSize {
			flush()
		}
		if curParts != nil {
			cur += sep
		}
		curParts = append(curParts, part{t, len(cur)})
		cur += t
	}
	flush()
	return chunks, parts
}

// within are the spans of fs inside [a, b), made relative to a; one that
// runs past either end, over into the text beside it, is cut at the end.
func within(fs []redact.Found, a, b int) []redact.Found {
	var out []redact.Found
	for _, f := range fs {
		s, e := max(f.Start, a), min(f.End, b)
		if s < e {
			out = append(out, redact.Found{Start: s - a, End: e - a, Label: f.Label, Score: f.Score})
		}
	}
	return out
}

// cache keeps what was found in the texts seen last, by their hashes.
var cache = newLRU(20000)

type lru struct {
	mu  sync.Mutex
	max int
	ll  *list.List
	m   map[[32]byte]*list.Element
}

type entry struct {
	k  [32]byte
	fs []redact.Found
}

func newLRU(n int) *lru { return &lru{max: n, ll: list.New(), m: map[[32]byte]*list.Element{}} }

func (c *lru) get(k [32]byte) ([]redact.Found, bool) {
	e, ok := c.m[k]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(e)
	return e.Value.(*entry).fs, true
}

func (c *lru) put(k [32]byte, fs []redact.Found) {
	if e, ok := c.m[k]; ok {
		c.ll.MoveToFront(e)
		e.Value.(*entry).fs = fs
		return
	}
	c.m[k] = c.ll.PushFront(&entry{k, fs})
	for c.ll.Len() > c.max {
		e := c.ll.Back()
		c.ll.Remove(e)
		delete(c.m, e.Value.(*entry).k)
	}
}
