// Package guard runs AstrLink Guard (QuantumNous/astrlink-guard,
// Apache-2.0), a token classifier for secrets and personal data in Chinese
// and English text: its int8 ONNX model (37 MB) read by onnx.go and run
// by bert.go on the CPU in Go, or on a Mac by CoreML (coreml.go),
// its tokenizer by tokenizer.go. Nothing is started.
package guard

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/awsl-project/maxx/internal/privacy/pii/guard/coreml"
)

// labels are the model's BIO tags, by output index (its labels.json).
var labels = []string{"O",
	"B-email", "I-email", "B-phone", "I-phone", "B-account", "I-account",
	"B-payment_card", "I-payment_card", "B-ip_address", "I-ip_address",
	"B-url", "I-url", "B-common_secret", "I-common_secret",
	"B-private_address", "I-private_address", "B-private_date", "I-private_date",
	"B-private_person", "I-private_person"}

// Files are the model's files Load reads, in its folder.
var Files = []string{"model_int8.onnx", "model_int8.onnx_data", "tokenizer.json"}

// Model is the model, loaded.
type Model struct {
	tok *tokenizer
	net *net
	ml  *coreml.Model // nil where CoreML isn't, or couldn't be made
	// MLErr is why CoreML isn't used, on a Mac.
	MLErr error
}

// Span is what the model found: text[Start:End], a Label of its (email,
// phone, account, payment_card, ip_address, url, common_secret,
// private_address, private_date, private_person), and the least of its
// tokens' probabilities.
type Span struct {
	Start, End int
	Label      string
	Score      float64
}

// MinScore is the publisher's default threshold, under which masking
// drops a span unless the user set another (internal/masking).
const MinScore = 0.8

// Floor is the probability under which Find drops a span: the lowest
// threshold a user can set.
const Floor = 0.5

// window is how many tokens the model reads at once, besides <s> and
// </s>; a longer text is read in windows overlapping by overlap, each
// token labelled by the window it is furthest inside.
const (
	window  = maxPos - 2
	overlap = 128
)

// Load reads the model in dir.
func Load(dir string) (*Model, error) {
	tok, err := loadTokenizer(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		return nil, fmt.Errorf("tokenizer: %w", err)
	}
	nt, err := loadNet(filepath.Join(dir, "model_int8.onnx"))
	if err != nil {
		return nil, fmt.Errorf("model: %w", err)
	}
	if nt.labels != len(labels) {
		return nil, fmt.Errorf("model: %d labels, not %d", nt.labels, len(labels))
	}
	m := &Model{tok: tok, net: nt}
	m.ml, m.MLErr = openCoreML(dir, nt)
	return m, nil
}

// logits are the network's scores for ids, from CoreML when it is there
// and the window is long enough to gain by it, else over procs goroutines.
func (m *Model) logits(ids []int, procs int) []float32 {
	if m.ml != nil && len(ids) >= coremlMin {
		if lg, err := coremlLogits(m.ml, m.net, ids); err == nil {
			return lg
		}
	}
	return m.net.logits(ids, procs)
}

// Find is what the model finds in text, at Floor, each span with its score.
// A text longer than a window is read a window at a time, overlapping,
// and on the CPU those windows are read at once, one a core: a window's
// own matmuls split over the cores leave most of each idle in the steps
// between them (a 4-core server read a long prompt for minutes).
func (m *Model) Find(text string) []Span {
	toks := m.tok.encode(text)
	n := len(toks)
	if n == 0 {
		return nil
	}
	var starts []int
	for start := 0; ; start = start + window - overlap {
		starts = append(starts, start)
		if start+window >= n {
			break
		}
	}
	lgs := make([][]float32, len(starts))
	read := func(w, procs int) {
		start := starts[w]
		end := min(start+window, n)
		ids := make([]int, 0, end-start+2)
		ids = append(ids, 0)
		for _, t := range toks[start:end] {
			ids = append(ids, t.id)
		}
		ids = append(ids, 2)
		lgs[w] = m.logits(ids, procs)
	}
	procs := runtime.GOMAXPROCS(0)
	at := min(procs, len(starts))
	if m.ml != nil || at == 1 {
		for w := range starts {
			read(w, threads)
		}
	} else {
		var next atomic.Int32
		var wg sync.WaitGroup
		for range at {
			wg.Go(func() {
				for w := int(next.Add(1)) - 1; w < len(starts); w = int(next.Add(1)) - 1 {
					read(w, max(1, procs/at))
				}
			})
		}
		wg.Wait()
	}
	lab := make([]int, n)
	prob := make([]float64, n)
	margin := make([]int, n)
	for i := range margin {
		margin[i] = -1
	}
	k := len(labels)
	for w, start := range starts {
		end := min(start+window, n)
		lg := lgs[w]
		for i := 0; i < end-start; i++ {
			j := start + i
			mg := min(i, end-start-1-i)
			if mg <= margin[j] {
				continue
			}
			row := lg[(i+1)*k : (i+2)*k]
			best, mx := 0, row[0]
			for c, v := range row {
				if v > mx {
					best, mx = c, v
				}
			}
			var sum float64
			for _, v := range row {
				sum += math.Exp(float64(v - mx))
			}
			margin[j], lab[j], prob[j] = mg, best, 1/sum
		}
	}
	return decode(toks, lab, prob)
}

// decode reads the tokens' BIO tags (lab, by index into labels) into
// spans: a B tag, or an I tag of another kind than the span before it,
// starts one; an I tag of the same kind extends it; O ends it. A span's
// score is its least token's, and those under Floor are dropped.
// Tokens of no text (a lone "▁") neither start nor end a span.
func decode(toks []token, lab []int, prob []float64) []Span {
	var spans []Span
	cur := -1
	for j, t := range toks {
		if t.start == t.end {
			continue
		}
		tag := labels[lab[j]]
		if tag == "O" {
			cur = -1
			continue
		}
		kind := tag[2:]
		if tag[0] == 'B' || cur < 0 || spans[cur].Label != kind {
			spans = append(spans, Span{t.start, t.end, kind, prob[j]})
			cur = len(spans) - 1
			continue
		}
		spans[cur].End = t.end
		spans[cur].Score = min(spans[cur].Score, prob[j])
	}
	out := spans[:0]
	for _, s := range spans {
		if s.Score >= Floor {
			out = append(out, s)
		}
	}
	return out
}
