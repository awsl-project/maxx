package privacy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/privacy/pii"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

type scopeKey struct{ tenant uint64 }

type tenantScope struct {
	state   *redact.State
	used    time.Time
	records ledger
}

// Runtime belongs to one Maxx database/runtime. Original values are never shared
// between its tenant scopes, or with another database in the process.
type Runtime struct {
	mu          sync.Mutex
	recordMu    sync.Mutex
	generations map[scopeKey]uint64
	scopes      map[scopeKey]*tenantScope
	directory   string
	find        func(context.Context, string, []string) ([][]redact.Found, error)
}

func newRuntime() *Runtime {
	return &Runtime{scopes: map[scopeKey]*tenantScope{}, generations: map[scopeKey]uint64{}, find: func(ctx context.Context, name string, texts []string) ([][]redact.Found, error) {
		return pii.Of(name).Find(ctx, texts)
	}}
}

// SetDirectory configures metadata/key persistence before serving requests.
// Merely configuring a directory creates no files or model processes.
func (s *Service) SetDirectory(directory string) { s.runtime.directory = directory }

func (r *Runtime) scope(key scopeKey) *tenantScope {
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope := r.scopes[key]; scope != nil {
		scope.used = time.Now()
		return scope
	}
	if len(r.scopes) >= 16 {
		var oldest scopeKey
		var when time.Time
		for k, scope := range r.scopes {
			if when.IsZero() || scope.used.Before(when) {
				oldest, when = k, scope.used
			}
		}
		// Existing exchanges retain their scope pointer until they finish.
		delete(r.scopes, oldest)
	}
	state := redact.NewState()
	state.SetLimits(4096, 8<<20)
	if r.directory != "" {
		state.SetKeyPath(filepath.Join(r.directory, strconv.FormatUint(key.tenant, 10), "placeholder.key"))
	}
	scope := &tenantScope{state: state, used: time.Now()}
	r.scopes[key] = scope
	return scope
}

// A nil Exchange means privacy is disabled; a non-nil exchange owns the policy
// snapshot for one HTTP request or WebSocket turn. Its restore map contains only
// values used by that exchange, even when the tenant's learned cache is shared.
type Exchange struct {
	Config           Config
	Body             []byte
	OriginalBody     []byte
	Hits             []redact.Hit
	Bypassed         bool
	key              scopeKey
	runtime          *Runtime
	scope            *tenantScope
	ctx              context.Context
	contentType      string
	rawBody          []byte
	rawOriginal      []byte
	pairs            map[string]string
	matched          map[string]bool
	evaluated        bool
	failed           bool
	rejection        *Rejection
	restored         map[string]int
	recordID         string
	recordTime       time.Time
	recordGeneration uint64
	recorded         bool
	tookUS           int64
}

func (s *Service) Prepare(ctx context.Context, tenantID uint64, body, original []byte, contentType string) (*Exchange, *Rejection) {
	if s == nil {
		return nil, nil
	}
	failure := func() (*Exchange, *Rejection) {
		rejection := checkFailure(http.StatusServiceUnavailable)
		return &Exchange{Body: body, OriginalBody: original, failed: true, evaluated: true, rejection: rejection}, rejection
	}

	cfg, err := s.Configuration()
	if err != nil {
		return failure()
	}
	if !cfg.Enabled {
		return nil, nil
	}
	if len(original) == 0 {
		original = body
	}
	exchange := &Exchange{Config: cfg, key: scopeKey{tenantID}, runtime: s.runtime,
		ctx: ctx, contentType: contentType, rawBody: body, rawOriginal: original, Body: body, OriginalBody: original,
		pairs: map[string]string{}, matched: map[string]bool{}}
	if len(cfg.BypassProviders) == 0 {
		return exchange, exchange.evaluate()
	}
	// Provider exceptions require the executor's resolved plan, before any
	// attempt is created. The router itself knows nothing about privacy.
	return exchange, nil
}

func blocked(kind string) *Rejection {
	return &Rejection{Code: "privacy_blocked", Message: "Request blocked: private content detected (" + kind + ")", HTTPStatusCode: http.StatusBadRequest}
}

// ResolvePlan follows Magpie's conservative fallback rule: bypass only when
// every provider that this request may reach is explicitly exempted.
func (e *Exchange) ResolvePlan(providerIDs []uint64) *Rejection {
	if e == nil {
		return nil
	}
	if e.evaluated {
		return e.rejection
	}
	if len(providerIDs) > 0 && len(e.Config.BypassProviders) > 0 {
		e.Bypassed = !slices.ContainsFunc(providerIDs, func(id uint64) bool { return !slices.Contains(e.Config.BypassProviders, id) })
		if e.Bypassed {
			e.evaluated = true
			return nil
		}
	}
	return e.evaluate()
}

func (e *Exchange) options() redact.Options {
	cfg := e.Config
	o := redact.Options{Secrets: cfg.Secrets, Personal: cfg.Personal, Kinds: cfg.Kinds, Words: cfg.Words,
		WordsAnyCase: cfg.WordsAnyCase, WordsWhole: cfg.WordsWhole, Rules: cfg.Rules, Actions: cfg.Actions,
		Allow: cfg.Allow, Shape: cfg.Shape, DefaultAction: cfg.Mode, NestedJSON: true, FieldKind: func(_, key, value string) string {
			if value == "" || value == Redacted || !sensitiveField(key, value) {
				return ""
			}
			key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			switch {
			case strings.Contains(key, "password"), strings.Contains(key, "passwd"):
				return "PASSWORD"
			case strings.Contains(key, "apikey"):
				return "API_KEY"
			case strings.Contains(key, "privatekey"):
				return "PRIVATE_KEY"
			case strings.Contains(key, "token"):
				return "TOKEN"
			default:
				return "SECRET"
			}
		}}
	if cfg.Model.Enabled && (cfg.Personal || cfg.Secrets) {
		threshold := cfg.Model.Score
		if threshold == 0 {
			threshold = pii.Of(cfg.Model.Name).MinScore()
		}
		o.MinScore = threshold
		o.NamesByModel = cfg.Model.Name == pii.AstrLinkGuard
		o.Model = func(texts []string) [][]redact.Found {
			found, err := e.runtime.find(e.ctx, cfg.Model.Name, texts)
			if err != nil {
				panic("local privacy model could not inspect request")
			}
			if len(found) != len(texts) {
				panic("local privacy model returned invalid results")
			}
			out := make([][]redact.Found, len(found))
			for i, spans := range found {
				for _, span := range spans {
					if span.Score >= threshold {
						out[i] = append(out[i], span)
					}
				}
			}
			return out
		}
	}
	// Count repeated occurrences in this payload, but do not recount content
	// already observed in the original or a previous converted payload.
	seen := make(map[[2]string]bool, len(e.Hits))
	for _, hit := range e.Hits {
		seen[[2]string{hit.Placeholder, hit.Kind}] = true
	}
	o.Seen = func(hit redact.Hit) {
		if !seen[[2]string{hit.Placeholder, hit.Kind}] {
			e.Hits = append(e.Hits, hit)
		}
	}
	o.Stored = func(placeholder, value string) {
		if _, known := e.pairs[placeholder]; !known && len(e.pairs) >= 4096 {
			panic("privacy request contains too many mappings")
		}
		e.pairs[placeholder] = value
	}
	o.Matched = func(_, value string) {
		if !e.matched[value] && len(e.matched) >= 4096 {
			panic("privacy request contains too many matches")
		}
		e.matched[value] = true
	}
	return o
}

// inspect is the single failure boundary for initial and converted payloads.
// Model errors, malformed bodies and mapping limits all fail closed here.
func (e *Exchange) inspect(body []byte) (masked []byte, rejection *Rejection) {
	started := time.Now()
	defer func() {
		e.tookUS += time.Since(started).Microseconds()
		if recover() != nil {
			e.failed = true
			masked = nil
			rejection = checkFailure(http.StatusServiceUnavailable)
		}
	}()
	if e.scope == nil {
		e.scope = e.runtime.scope(e.key)
	}
	masked, err := maskRequest(e.scope.state, body, e.contentType, e.options())
	if err != nil {
		e.failed = true
		return nil, checkFailure(http.StatusBadRequest)
	}
	return masked, nil
}

func (e *Exchange) evaluate() (rejection *Rejection) {
	if e.evaluated {
		return e.rejection
	}
	e.evaluated = true
	defer func() { e.rejection = rejection }()
	original, rejection := e.inspect(e.rawOriginal)
	if rejection != nil {
		return rejection
	}
	body := original
	if !bytes.Equal(e.rawOriginal, e.rawBody) {
		body, rejection = e.inspect(e.rawBody)
		if rejection != nil {
			return rejection
		}
	}
	e.Body, e.OriginalBody = body, original
	return blockingRejection(e.Hits)
}

func blockingRejection(hits []redact.Hit) *Rejection {
	for _, hit := range hits {
		if hit.Action == redact.Block {
			return blocked(hit.Kind)
		}
	}
	return nil
}

func maskRequest(state *redact.State, body []byte, contentType string, options redact.Options) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return nil, errors.New("invalid content type")
	}
	if mediaType == "multipart/form-data" {
		var buffer bytes.Buffer
		writer := multipart.NewWriter(&buffer)
		if err := writer.SetBoundary(params["boundary"]); err != nil {
			return nil, err
		}
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		changed := false
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			target, err := writer.CreatePart(part.Header)
			if err != nil {
				return nil, err
			}
			if part.FileName() != "" {
				if _, err := io.Copy(target, part); err != nil {
					return nil, err
				}
				continue
			}
			value, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			masked, n := state.Mask(string(value), options)
			changed = changed || n > 0
			io.WriteString(target, masked)
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		if !changed {
			return body, nil
		}
		return buffer.Bytes(), nil
	}
	if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return nil, errors.New("unsupported body type")
	}
	value, err := decode(string(body))
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("expected JSON request object")
	}
	masked, count := state.MaskJSON(body, options)
	if count == 0 {
		return body, nil
	}
	return masked, nil
}

func (e *Exchange) RestoreState() *redact.State { return redact.Restorer(e.pairs) }

func (e *Exchange) HasMappings() bool { return e != nil && !e.Bypassed && len(e.pairs) > 0 }

// Outbound inspects fields added by provider parameter policies/conversion.
// It runs before creating an attempt, and retains mappings for this response.
func (e *Exchange) Outbound(body []byte) ([]byte, *Rejection) {
	if e == nil || e.Bypassed {
		return body, nil
	}
	if !e.evaluated || e.failed {
		return nil, checkFailure(http.StatusServiceUnavailable)
	}
	if e.rejection != nil {
		return nil, e.rejection
	}
	start := len(e.Hits)
	masked, rejection := e.inspect(body)
	if rejection != nil {
		return nil, rejection
	}
	return masked, blockingRejection(e.Hits[start:])
}
