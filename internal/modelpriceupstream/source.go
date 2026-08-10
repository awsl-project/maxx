package modelpriceupstream

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

const DefaultSourceCode = "litellm"

var ErrUnsupportedSource = errors.New("unsupported model price upstream source")

// SourceInfo is source metadata returned with normalized upstream prices.
type SourceInfo struct {
	Code string
	Name string
}

// Source fetches model prices from one upstream and normalizes them into
// Maxx's unified domain.ModelPrice shape. Implementations own their upstream
// protocol, auth, payload shape, and conversion rules.
type Source interface {
	Code() string
	Name() string
	Fetch() ([]*domain.ModelPrice, string, error)
}

var sources = map[string]Source{
	DefaultSourceCode: NewLiteLLMSource(),
}

// Register adds or replaces a model price upstream source implementation.
func Register(source Source) error {
	if source == nil {
		return fmt.Errorf("model price upstream source is nil")
	}
	code := normalizeSourceCode(source.Code())
	if code == "" {
		return fmt.Errorf("model price upstream source code is empty")
	}
	sources[code] = source
	return nil
}

// Resolve returns the configured source, defaulting to LiteLLM for backwards compatibility.
func Resolve(sourceCode string) (Source, error) {
	code := normalizeSourceCode(sourceCode)
	if code == "" {
		code = DefaultSourceCode
	}
	source, ok := sources[code]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnsupportedSource, sourceCode)
	}
	return source, nil
}

// Fetch fetches and normalizes model prices from the requested source.
func Fetch(sourceCode string) ([]*domain.ModelPrice, SourceInfo, string, error) {
	source, err := Resolve(sourceCode)
	if err != nil {
		return nil, SourceInfo{}, "", err
	}
	prices, sourceURL, err := source.Fetch()
	if err != nil {
		return nil, SourceInfo{Code: source.Code(), Name: source.Name()}, sourceURL, err
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i].ModelID < prices[j].ModelID })
	return prices, SourceInfo{Code: source.Code(), Name: source.Name()}, sourceURL, nil
}

func normalizeSourceCode(code string) string {
	return strings.TrimSpace(strings.ToLower(code))
}
