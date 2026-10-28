package privacy

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy/pii"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

type scopeKey struct{ tenant, project uint64 }

type projectScope struct {
	state   *redact.State
	used    time.Time
	records ledger
}

// Runtime belongs to one Maxx database/runtime. Original values are never shared
// between its tenant/project scopes, or with another database in the process.
type Runtime struct {
	mu          sync.Mutex
	recordMu    sync.Mutex
	generations map[scopeKey]uint64
	scopes      map[scopeKey]*projectScope
	directory   string
	find        func(context.Context, string, []string) ([][]redact.Found, error)
}

func newRuntime() *Runtime {
	return &Runtime{scopes: map[scopeKey]*projectScope{}, generations: map[scopeKey]uint64{}, find: func(ctx context.Context, name string, texts []string) ([][]redact.Found, error) {
		return pii.Of(name).Find(ctx, texts)
	}}
}

// SetDirectory configures metadata/key persistence before serving requests.
// Merely configuring a directory creates no files or model processes.
func (s *Service) SetDirectory(directory string) { s.runtime.directory = directory }

func (r *Runtime) scope(key scopeKey) *projectScope {
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
		state.SetKeyPath(filepath.Join(r.directory, strconv.FormatUint(key.tenant, 10), strconv.FormatUint(key.project, 10), "placeholder.key"))
	}
	scope := &projectScope{state: state, used: time.Now()}
	r.scopes[key] = scope
	return scope
}

// Exchange is one HTTP request or WebSocket turn. Its restore map contains only
// values used by that exchange, even when the project's learned cache is shared.
type Exchange struct {
	Enabled          bool
	Config           Config
	Body             []byte
	OriginalBody     []byte
	Hits             []redact.Hit
	Bypassed         bool
	key              scopeKey
	runtime          *Runtime
	scope            *projectScope
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

func (s *Service) Prepare(ctx context.Context, tenantID, projectID uint64, body, original []byte, contentType string) (*Exchange, *Rejection) {
	if projectID == 0 {
		return nil, nil
	}
	failure := func() (*Exchange, *Rejection) {
		rejection := checkFailure(http.StatusServiceUnavailable)
		return &Exchange{Enabled: true, Body: body, OriginalBody: original, failed: true, evaluated: true, rejection: rejection}, rejection
	}

	if s == nil || s.projects == nil {
		return failure()
	}
	project, err := s.projects.GetByID(tenantID, projectID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil || project == nil {
		return failure()
	}
	if !project.BlockSecrets {
		return nil, nil // Off bypasses configuration/body parsing and runtime work.
	}
	cfg, err := projectConfig(project)
	if err != nil {
		return failure()
	}
	if len(original) == 0 {
		original = body
	}
	exchange := &Exchange{Enabled: true, Config: cfg, key: scopeKey{tenantID, projectID}, runtime: s.runtime,
		ctx: ctx, contentType: contentType, rawBody: body, rawOriginal: original, Body: body, OriginalBody: original,
		pairs: map[string]string{}, matched: map[string]bool{}}
	if len(project.Privacy) == 0 || bytes.Equal(project.Privacy, []byte("null")) {
		// Existing boolean-only clients retain the original blocking semantics.
		exchange.evaluated = true
		kind, err := InspectRequest(original, contentType)
		if err != nil {
			exchange.failed = true
			exchange.rejection = checkFailure(http.StatusBadRequest)
		}
		if kind != "" {
			exchange.rejection = blocked(kind)
		}
		return exchange, exchange.rejection
	}
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
	if e == nil || e.evaluated {
		if e == nil {
			return nil
		}
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
	o.Seen = func(hit redact.Hit) { e.Hits = append(e.Hits, hit) }
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

func (e *Exchange) evaluate() (rejection *Rejection) {
	if e.evaluated {
		return e.rejection
	}
	e.evaluated = true
	started := time.Now()
	defer func() {
		e.tookUS += time.Since(started).Microseconds()
		if recover() != nil {
			e.failed = true
			rejection = checkFailure(http.StatusServiceUnavailable)
		}
		e.rejection = rejection
	}()
	e.scope = e.runtime.scope(e.key)
	o := e.options()
	original, err := maskRequest(e.scope.state, e.rawOriginal, e.contentType, o)
	if err != nil {
		e.failed = true
		return checkFailure(http.StatusBadRequest)
	}
	body := original
	if !bytes.Equal(e.rawOriginal, e.rawBody) {
		// Converters may have retained a second form of the same message.
		// Count that content once, but still inspect any newly injected values.
		seen := map[string]bool{}
		for _, hit := range e.Hits {
			seen[hit.Placeholder+"|"+hit.Kind] = true
		}
		o.Seen = func(hit redact.Hit) {
			if !seen[hit.Placeholder+"|"+hit.Kind] {
				e.Hits = append(e.Hits, hit)
			}
		}
		body, err = maskRequest(e.scope.state, e.rawBody, e.contentType, o)
		if err != nil {
			e.failed = true
			return checkFailure(http.StatusBadRequest)
		}
	}
	e.Body, e.OriginalBody = body, original
	for _, hit := range e.Hits {
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
func (e *Exchange) Outbound(body []byte) (masked []byte, rejection *Rejection) {
	if e == nil || e.Bypassed || e.scope == nil {
		return body, nil
	}
	defer func() {
		if recover() != nil {
			e.failed = true
			masked = nil
			rejection = checkFailure(http.StatusServiceUnavailable)
		}
	}()
	start := len(e.Hits)
	o := e.options()
	seen := map[string]bool{}
	for _, hit := range e.Hits {
		seen[hit.Placeholder+"|"+hit.Kind] = true
	}
	o.Seen = func(hit redact.Hit) {
		if !seen[hit.Placeholder+"|"+hit.Kind] {
			e.Hits = append(e.Hits, hit)
		}
	}
	var err error
	masked, err = maskRequest(e.scope.state, body, e.contentType, o)
	if err != nil {
		e.failed = true
		return nil, checkFailure(http.StatusBadRequest)
	}
	for _, hit := range e.Hits[start:] {
		if hit.Action == redact.Block {
			return nil, blocked(hit.Kind)
		}
	}
	return masked, nil
}

// MarshalConfig is used by policy endpoints without exposing runtime state.
func MarshalConfig(cfg Config) json.RawMessage { data, _ := json.Marshal(cfg); return data }
