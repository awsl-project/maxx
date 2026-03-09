package usage

import "testing"

func TestExtractFromSSELineCodexResponseCompleted(t *testing.T) {
	line := `data: {"type":"response.completed","response":{"usage":{"input_tokens":120,"output_tokens":34,"input_tokens_details":{"cached_tokens":11}}}}`

	metrics := ExtractFromSSELine(line)
	if metrics == nil {
		t.Fatal("expected metrics")
	}
	if metrics.InputTokens != 120 || metrics.OutputTokens != 34 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
	if metrics.CacheReadCount != 11 {
		t.Fatalf("expected cached tokens, got %+v", metrics)
	}
}

func TestExtractFromSSELineClaudeMessageDelta(t *testing.T) {
	line := `data: {"type":"message_delta","usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":2}}`

	metrics := ExtractFromSSELine(line)
	if metrics == nil {
		t.Fatal("expected metrics")
	}
	if metrics.InputTokens != 10 || metrics.OutputTokens != 4 || metrics.CacheReadCount != 2 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}

func TestExtractFromSSELineIgnoresNonData(t *testing.T) {
	if metrics := ExtractFromSSELine("event: message"); metrics != nil {
		t.Fatalf("expected nil metrics, got %+v", metrics)
	}
}
