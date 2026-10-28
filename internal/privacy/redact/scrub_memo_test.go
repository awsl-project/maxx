package redact

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// sourceLines are what a shell call reads back while an agent works on a
// gateway: code that names tokens, keys and URLs, as the rules look for.
const sourceLines = `// step %d
func (c *Client) refresh(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://auth.example.com/oauth/token", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.apiKey == "" { return "", errors.New("no api key") }
	var out struct{ AccessToken string ` + "`json:\"access_token\"`" + `; ExpiresIn int }
	return out.AccessToken, json.NewDecoder(resp.Body).Decode(&out) // see github.com/example/auth
}
`

// codexHistory is a Responses request as Codex sends one on turn n: every
// item of the session so far, each turn's tool output a few KB.
func codexHistory(n int) []byte {
	var items []string
	for i := 0; i < n; i++ {
		items = append(items,
			fmt.Sprintf(`{"type":"message","role":"user","content":[{"type":"input_text","text":"step %d: look at the build log and fix what failed, then run the tests again"}]}`, i),
			fmt.Sprintf(`{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"go test ./internal/... 2>&1 | tail -n 200\"]}","call_id":"call_%d"}`, i),
			fmt.Sprintf(`{"type":"function_call_output","call_id":"call_%d","output":%q}`, i, strings.Repeat(fmt.Sprintf(sourceLines, i), 8)),
		)
	}
	return []byte(`{"model":"gpt-6-astra","instructions":"You are a coding agent.","input":[` + strings.Join(items, ",") + `],"stream":true}`)
}

// TestScrubRunsTheRulesOnceAString: a history sent again on the next turn
// has the rules run only over what it did not have before
// (yetone/magpie-releases#24), and comes out as it did.
func TestScrubRunsTheRulesOnceAString(t *testing.T) {
	defaultState.scrubMemo.reset()
	first := codexHistory(40)
	want := ScrubJSON(first)
	runs := defaultState.scrubRuns.Load()
	next := codexHistory(41)
	got := ScrubJSON(next)
	// the new turn's one long string, its tool's output
	if n := defaultState.scrubRuns.Load() - runs; n != 1 {
		t.Fatalf("the rules ran over %d strings for one new turn", n)
	}
	if !strings.HasPrefix(string(got), string(want[:len(want)-len(`],"stream":true}`)])) {
		t.Fatal("the history scrubbed again is not what it was")
	}
	defaultState.scrubMemo.reset()
	if again := ScrubJSON(next); string(again) != string(got) {
		t.Fatal("remembered and fresh scrubs differ")
	}
}

// TestScrubMemoForgetsWhenAValueBecomesKnown: a string remembered as having
// nothing to scrub is scrubbed again once a value in it is masked
// somewhere else: what is known is part of what it has to lose.
func TestScrubMemoForgetsWhenAValueBecomesKnown(t *testing.T) {
	// a value no run before this one made known
	secret := fmt.Sprintf("Qz8marmot%dPass", time.Now().UnixNano())
	long := strings.Repeat("the build log says nothing of note here. ", 10) + "we named the box " + secret + " and that is all"
	if got := Scrub(long); got != long {
		t.Fatalf("scrubbed before it was known: %q", got)
	}
	Mask("DB_PASSWORD="+secret, Options{Secrets: true})
	if got := Scrub(long); strings.Contains(got, secret) || !strings.Contains(got, "[REDACTED:") {
		t.Fatalf("a value known since was left in: %q", got)
	}
	// and through a JSON body as the conversation records it
	body := fmt.Sprintf(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}]}`, long)
	if got := ScrubJSON([]byte(body)); strings.Contains(string(got), secret) {
		t.Fatalf("%s", got)
	}
}

// TestScrubMemoKeepsTheSecretOut: a long string with a secret in it comes
// out scrubbed every time, the remembered result as the first.
func TestScrubMemoKeepsTheSecretOut(t *testing.T) {
	const key = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"
	long := strings.Repeat("a line of the log with nothing in it\n", 10) + "export ANTHROPIC_API_KEY=" + key + "\n"
	first := Scrub(long)
	if strings.Contains(first, key) {
		t.Fatalf("%q", first)
	}
	for i := 0; i < 3; i++ {
		if got := Scrub(long); got != first {
			t.Fatalf("turn %d: %q", i, got)
		}
	}
}

// TestScrubMemoBounded: the memo holds at most two generations of its
// bounds, however many strings go through it.
func TestScrubMemoBounded(t *testing.T) {
	defaultState.scrubMemo.reset()
	t.Cleanup(defaultState.scrubMemo.reset)
	pad := strings.Repeat("x", scrubMemoMin)
	for i := 0; i < 2*scrubMemoEntries+10; i++ {
		Scrub(fmt.Sprintf("%d %s", i, pad))
	}
	if n := len(defaultState.scrubMemo.cur) + len(defaultState.scrubMemo.old); n > 2*scrubMemoEntries {
		t.Fatalf("%d entries held", n)
	}
}

// BenchmarkScrubCodexTurn is one Codex turn recorded: the request, its
// whole history, scrubbed after the turn before it was.
func BenchmarkScrubCodexTurn(b *testing.B) {
	prev, next := codexHistory(300), codexHistory(301)
	ScrubJSON(prev)
	b.SetBytes(int64(len(next)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScrubJSON(next)
	}
}
