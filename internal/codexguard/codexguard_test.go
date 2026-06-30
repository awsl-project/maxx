package codexguard

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if !reflect.DeepEqual(cfg.BlockedReasoningTokens, []int{516, 1034, 1552}) {
		t.Fatalf("BlockedReasoningTokens = %#v, want %#v", cfg.BlockedReasoningTokens, []int{516, 1034, 1552})
	}
	if cfg.MaxAttempts != 2 {
		t.Fatalf("MaxAttempts = %d, want 2", cfg.MaxAttempts)
	}
	if cfg.StatusCode != 502 {
		t.Fatalf("StatusCode = %d, want 502", cfg.StatusCode)
	}
	if cfg.ErrorCode != "reasoning_guard_triggered" {
		t.Fatalf("ErrorCode = %q, want reasoning_guard_triggered", cfg.ErrorCode)
	}
	if cfg.Mode != "non_stream" {
		t.Fatalf("Mode = %q, want non_stream", cfg.Mode)
	}

	cfg.BlockedReasoningTokens[0] = 999
	next := DefaultConfig()
	if next.BlockedReasoningTokens[0] != 516 {
		t.Fatalf("DefaultConfig returned shared blocked token slice")
	}
}

func TestParseConfigJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Config
		wantErr bool
	}{
		{
			name:  "empty uses defaults",
			input: "",
			want:  DefaultConfig(),
		},
		{
			name: "partial override",
			input: `{
				"enabled": false,
				"blocked_reasoning_tokens": [1, 2],
				"max_attempts": 3,
				"status_code": 503,
				"error_code": "custom_guard",
				"mode": "non_stream"
			}`,
			want: Config{
				Enabled:                false,
				BlockedReasoningTokens: []int{1, 2},
				MaxAttempts:            3,
				StatusCode:             503,
				ErrorCode:              "custom_guard",
				Mode:                   ModeNonStream,
			},
		},
		{
			name:    "invalid json",
			input:   `{`,
			wantErr: true,
		},
		{
			name:    "invalid config",
			input:   `{"max_attempts":0}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfigJSON([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseConfigJSON returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseConfigJSON returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseConfigJSON = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	valid := DefaultConfig()
	if err := ValidateConfig(valid); err != nil {
		t.Fatalf("ValidateConfig(valid) returned error: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "max attempts",
			mutate: func(cfg *Config) {
				cfg.MaxAttempts = 0
			},
		},
		{
			name: "status code",
			mutate: func(cfg *Config) {
				cfg.StatusCode = 200
			},
		},
		{
			name: "error code",
			mutate: func(cfg *Config) {
				cfg.ErrorCode = " "
			},
		},
		{
			name: "mode",
			mutate: func(cfg *Config) {
				cfg.Mode = "batch"
			},
		},
		{
			name: "blocked token",
			mutate: func(cfg *Config) {
				cfg.BlockedReasoningTokens = []int{-1}
			},
		},
		{
			name: "empty blocked token",
			mutate: func(cfg *Config) {
				cfg.BlockedReasoningTokens = nil
			},
		},
		{
			name: "duplicate blocked token",
			mutate: func(cfg *Config) {
				cfg.BlockedReasoningTokens = []int{516, 516}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			if err := ValidateConfig(cfg); err == nil {
				t.Fatal("ValidateConfig returned nil error")
			}
		})
	}
}

func TestExtractReasoningTokensFromJSON(t *testing.T) {
	payload := []byte(`{
		"id": "resp_123",
		"usage": {
			"output_tokens_details": {
				"reasoning_tokens": 516
			}
		},
		"choices": [
			{
				"message": {
					"usage": {
						"completion_tokens_details": {
							"reasoning_tokens": "1034"
						}
					}
				}
			}
		],
		"nested": {
			"reasoning_tokens": [1552, "7", null]
		}
	}`)

	got, err := ExtractReasoningTokensFromJSON(payload)
	if err != nil {
		t.Fatalf("ExtractReasoningTokensFromJSON returned error: %v", err)
	}
	want := []int{516, 1034, 1552, 7}
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractReasoningTokensFromJSON = %#v, want %#v", got, want)
	}
}

func TestExtractReasoningTokensFromJSONNoMatch(t *testing.T) {
	got, err := ExtractReasoningTokensFromJSON([]byte(`{"usage":{"output_tokens":5}}`))
	if err != nil {
		t.Fatalf("ExtractReasoningTokensFromJSON returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ExtractReasoningTokensFromJSON = %#v, want empty slice", got)
	}
}

func TestExtractReasoningTokensFromJSONInvalidReasoningToken(t *testing.T) {
	if _, err := ExtractReasoningTokensFromJSON([]byte(`{"reasoning_tokens":"1.5"}`)); err == nil {
		t.Fatal("ExtractReasoningTokensFromJSON returned nil error")
	}
}

func TestIsBlockedToken(t *testing.T) {
	blocked := []int{516, 1034, 1552}

	if !IsBlockedToken(1034, blocked) {
		t.Fatal("IsBlockedToken(1034) = false, want true")
	}
	if IsBlockedToken(42, blocked) {
		t.Fatal("IsBlockedToken(42) = true, want false")
	}
}

func TestSSEInspectorMultipleData(t *testing.T) {
	inspector, err := NewSSEInspector(DefaultConfig())
	if err != nil {
		t.Fatalf("NewSSEInspector returned error: %v", err)
	}

	stream := "" +
		"event: response.output_text.delta\n" +
		"data: {\"usage\":{\"output_tokens_details\":{\"reasoning_tokens\":42}}}\n\n" +
		"data: [DONE]\n" +
		"data: {\"choices\":[{\"usage\":{\"completion_tokens_details\":{\"reasoning_tokens\":\"1034\"}}}]}\n"

	err = inspector.Inspect([]byte(stream))
	if err == nil {
		t.Fatal("Inspect returned nil error")
	}
	if !IsReasoningGuardError(err) {
		t.Fatalf("Inspect error = %v, want reasoning guard error", err)
	}

	var guardErr ReasoningGuardError
	if !errors.As(err, &guardErr) {
		t.Fatalf("errors.As failed for %T", err)
	}
	if guardErr.Token != 1034 {
		t.Fatalf("guardErr.Token = %d, want 1034", guardErr.Token)
	}
}

func TestSSEInspectorNoHitAndFlush(t *testing.T) {
	inspector, err := NewSSEInspector(DefaultConfig())
	if err != nil {
		t.Fatalf("NewSSEInspector returned error: %v", err)
	}

	if err := inspector.Inspect([]byte("data: {\"reasoning_tokens\":42}")); err != nil {
		t.Fatalf("Inspect returned error before complete line: %v", err)
	}
	if err := inspector.Flush(); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
}

func TestSSEInspectorDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false

	inspector, err := NewSSEInspector(cfg)
	if err != nil {
		t.Fatalf("NewSSEInspector returned error: %v", err)
	}
	if err := inspector.Inspect([]byte("data: {\"reasoning_tokens\":516}\n")); err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
}

func TestReasoningGuardErrorSentinel(t *testing.T) {
	err := NewReasoningGuardError(516, DefaultConfig())

	if !IsReasoningGuardError(err) {
		t.Fatal("IsReasoningGuardError returned false")
	}
	if !errors.Is(err, ErrReasoningGuardTriggered) {
		t.Fatal("errors.Is(err, ErrReasoningGuardTriggered) returned false")
	}
	if IsReasoningGuardError(errors.New("other")) {
		t.Fatal("IsReasoningGuardError(other) returned true")
	}
}
