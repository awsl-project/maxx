package codexguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	ModeNonStream = "non_stream"

	DefaultStatusCode = 502
	DefaultErrorCode  = "reasoning_guard_triggered"
)

var (
	defaultBlockedReasoningTokens = []int{516, 1034, 1552}

	ErrReasoningGuardTriggered = errors.New("reasoning guard triggered")
)

type Config struct {
	Enabled                bool   `json:"enabled"`
	BlockedReasoningTokens []int  `json:"blocked_reasoning_tokens"`
	MaxAttempts            int    `json:"max_attempts"`
	StatusCode             int    `json:"status_code"`
	ErrorCode              string `json:"error_code"`
	Mode                   string `json:"mode"`
}

func DefaultConfig() Config {
	blocked := append([]int(nil), defaultBlockedReasoningTokens...)

	return Config{
		Enabled:                true,
		BlockedReasoningTokens: blocked,
		MaxAttempts:            2,
		StatusCode:             DefaultStatusCode,
		ErrorCode:              DefaultErrorCode,
		Mode:                   ModeNonStream,
	}
}

func ParseConfigJSON(data []byte) (Config, error) {
	cfg := DefaultConfig()
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse codexguard config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Config{}, fmt.Errorf("parse codexguard config: %w", err)
		}
		return Config{}, errors.New("parse codexguard config: trailing JSON value")
	}
	if err := ValidateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ValidateConfig(cfg Config) error {
	if cfg.MaxAttempts < 1 {
		return fmt.Errorf("invalid max_attempts %d: must be >= 1", cfg.MaxAttempts)
	}
	if cfg.StatusCode < 400 || cfg.StatusCode > 599 {
		return fmt.Errorf("invalid status_code %d: must be in [400, 599]", cfg.StatusCode)
	}
	if strings.TrimSpace(cfg.ErrorCode) == "" {
		return errors.New("invalid error_code: must not be empty")
	}
	if cfg.Mode != ModeNonStream {
		return fmt.Errorf("invalid mode %q: must be %q", cfg.Mode, ModeNonStream)
	}
	if cfg.MaxAttempts > 10 {
		return fmt.Errorf("invalid max_attempts %d: must be <= 10", cfg.MaxAttempts)
	}
	if cfg.Enabled && len(cfg.BlockedReasoningTokens) == 0 {
		return errors.New("invalid blocked_reasoning_tokens: must not be empty when enabled")
	}
	seen := make(map[int]struct{}, len(cfg.BlockedReasoningTokens))
	for _, token := range cfg.BlockedReasoningTokens {
		if token < 0 {
			return fmt.Errorf("invalid blocked_reasoning_tokens entry %d: must be >= 0", token)
		}
		if _, ok := seen[token]; ok {
			return fmt.Errorf("invalid blocked_reasoning_tokens entry %d: duplicate value", token)
		}
		seen[token] = struct{}{}
	}

	return nil
}

func ExtractReasoningTokensFromJSON(data []byte) ([]int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var payload any
	if err := dec.Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse reasoning token JSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("parse reasoning token JSON: trailing data")
	}

	var tokens []int
	if err := collectReasoningTokens(payload, &tokens); err != nil {
		return nil, err
	}

	return tokens, nil
}

func collectReasoningTokens(value any, tokens *[]int) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "reasoning_tokens" {
				if err := appendReasoningTokenValue(nested, tokens); err != nil {
					return err
				}
			}
			if err := collectReasoningTokens(nested, tokens); err != nil {
				return err
			}
		}
	case []any:
		for _, nested := range typed {
			if err := collectReasoningTokens(nested, tokens); err != nil {
				return err
			}
		}
	}

	return nil
}

func appendReasoningTokenValue(value any, tokens *[]int) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case json.Number:
		token, err := parseReasoningTokenNumber(typed.String())
		if err != nil {
			return err
		}
		*tokens = append(*tokens, token)
	case string:
		token, err := parseReasoningTokenNumber(strings.TrimSpace(typed))
		if err != nil {
			return err
		}
		*tokens = append(*tokens, token)
	case []any:
		for _, nested := range typed {
			if err := appendReasoningTokenValue(nested, tokens); err != nil {
				return err
			}
		}
	case map[string]any:
		return collectReasoningTokens(typed, tokens)
	default:
		return fmt.Errorf("invalid reasoning_tokens value %T: want integer number or integer string", value)
	}

	return nil
}

func parseReasoningTokenNumber(raw string) (int, error) {
	if raw == "" {
		return 0, errors.New("invalid reasoning_tokens value: empty string")
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid reasoning_tokens value %q: %w", raw, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("invalid reasoning_tokens value %d: must be >= 0", parsed)
	}
	if parsed > int64(math.MaxInt) {
		return 0, fmt.Errorf("invalid reasoning_tokens value %d: overflows int", parsed)
	}

	return int(parsed), nil
}

func IsBlockedToken(token int, blocked []int) bool {
	for _, candidate := range blocked {
		if token == candidate {
			return true
		}
	}

	return false
}

type ReasoningGuardError struct {
	Token      int
	StatusCode int
	ErrorCode  string
}

func NewReasoningGuardError(token int, cfg Config) ReasoningGuardError {
	return ReasoningGuardError{
		Token:      token,
		StatusCode: cfg.StatusCode,
		ErrorCode:  cfg.ErrorCode,
	}
}

func (e ReasoningGuardError) Error() string {
	return fmt.Sprintf("%s: reasoning_tokens %d is blocked", e.ErrorCode, e.Token)
}

func (e ReasoningGuardError) Unwrap() error {
	return ErrReasoningGuardTriggered
}

func IsReasoningGuardError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrReasoningGuardTriggered) {
		return true
	}

	var guardErr ReasoningGuardError
	return errors.As(err, &guardErr)
}

type SSEInspector struct {
	cfg Config
	buf string
}

func NewSSEInspector(cfg Config) (*SSEInspector, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}

	return &SSEInspector{cfg: cfg}, nil
}

func (i *SSEInspector) Inspect(chunk []byte) error {
	if i == nil {
		return errors.New("nil SSEInspector")
	}
	if !i.cfg.Enabled {
		return nil
	}

	i.buf += string(chunk)
	for {
		idx := strings.IndexByte(i.buf, '\n')
		if idx < 0 {
			return nil
		}

		line := i.buf[:idx]
		i.buf = i.buf[idx+1:]
		if err := i.InspectLine(line); err != nil {
			return err
		}
	}
}

func (i *SSEInspector) Flush() error {
	if i == nil {
		return errors.New("nil SSEInspector")
	}
	if !i.cfg.Enabled {
		i.buf = ""
		return nil
	}
	if i.buf == "" {
		return nil
	}

	line := i.buf
	i.buf = ""
	return i.InspectLine(line)
}

func (i *SSEInspector) InspectLine(line string) error {
	if i == nil {
		return errors.New("nil SSEInspector")
	}
	if !i.cfg.Enabled {
		return nil
	}

	line = strings.TrimRight(line, "\r")
	if !strings.HasPrefix(line, "data:") {
		return nil
	}

	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return nil
	}

	tokens, err := ExtractReasoningTokensFromJSON([]byte(data))
	if err != nil {
		return fmt.Errorf("inspect SSE data: %w", err)
	}
	for _, token := range tokens {
		if IsBlockedToken(token, i.cfg.BlockedReasoningTokens) {
			return NewReasoningGuardError(token, i.cfg)
		}
	}

	return nil
}
