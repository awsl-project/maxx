package jsonutil

import (
	stdjson "encoding/json"
	"strings"
	"testing"
)

func TestUnmarshalRawMessageOwnsData(t *testing.T) {
	input := []byte(`{"value":{"text":"one"}}`)
	var object map[string]stdjson.RawMessage
	if err := Unmarshal(input, &object); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	copy(input, []byte(`{"value":{"text":"two"}}`))
	if got := string(object["value"]); got != `{"text":"one"}` {
		t.Fatalf("raw message changed with input buffer: %s", got)
	}
}

func TestUnmarshalString(t *testing.T) {
	var value struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := UnmarshalString(`{"model":"gpt-test","stream":true}`, &value); err != nil {
		t.Fatalf("UnmarshalString: %v", err)
	}
	if value.Model != "gpt-test" || !value.Stream {
		t.Fatalf("decoded value = %+v", value)
	}
}

func BenchmarkUnmarshalLargeRawMessage(b *testing.B) {
	input := []byte(`{"model":"gpt-test","input":[{"content":"` + strings.Repeat("x", 1<<20) + `"}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		var object map[string]stdjson.RawMessage
		if err := Unmarshal(input, &object); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalLargeObject(b *testing.B) {
	input := []byte(`{"type":"response.completed","response":{"model":"gpt-test","output":[{"content":"` + strings.Repeat("x", 1<<20) + `"}]}}`)
	b.Run("Sonic", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for i := 0; i < b.N; i++ {
			var object map[string]interface{}
			if err := Unmarshal(input, &object); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("EncodingJSON", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(input)))
		for i := 0; i < b.N; i++ {
			var object map[string]interface{}
			if err := stdjson.Unmarshal(input, &object); err != nil {
				b.Fatal(err)
			}
		}
	})
}
