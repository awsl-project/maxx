package redact

import (
	"strings"
	"testing"
)

// A hash's hex digits short of 32 are an id, as 32 or more are: under a
// bare "key", a cache's key, a field named for a hash or as a JSON key of
// data keyed by hashes they go as written, the rules' and the model's
// both (_kaichen on X, 10-28: "hash类型也会被过滤，有些以hash为key的数据，
// 被当作key被mask"). Under a credential's name, or after a key's name in
// a sentence, the same digits are a key still.
func TestShortDigestsGoAsWritten(t *testing.T) {
	Forget()
	defer Forget()
	o := Options{Secrets: true, Personal: true}
	kept := []string{
		`{"key": "5d41402abc4b2a76"}`,
		"key: 1a2b3c4d5e6f7a8b9c0d",                  // gitleaks:allow -- public upstream regression fixture or detector example
		`{"key":"0123456789abcdef0123"}`,             // gitleaks:allow -- public upstream regression fixture or detector example
		`{"token_hash": "0123456789abcdef01234567"}`, // gitleaks:allow -- public upstream regression fixture or detector example
		`{"cache_key": "a1b2c3d4e5f60718293a"}`,      // gitleaks:allow -- public upstream regression fixture or detector example
		`cacheKey = "a1b2c3d4e5f60718293b"`,          // gitleaks:allow -- public upstream regression fixture or detector example
		`{"object_key": "A1B2C3D4E5F60718293C"}`,     // gitleaks:allow -- public upstream regression fixture or detector example
		`{"hash": "9f13800138000abc"}`,
		`{"keys": {"5d41402abc4b2a76": {"hits": 3}, "7c4a8d09ca3762af": {"hits": 1}}}`,
		"git checkout 5d41402abc4b2a76e",
	}
	for _, s := range kept {
		if out := Preview(s, o); out != s {
			t.Errorf("%q masked: %q", s, out)
		}
	}
	masked := []struct{ s, v string }{
		{"password=deadbeef12345678", "deadbeef12345678"},
		{"api_key: 9f2c4e6a8b0d1f3e5a7c", "9f2c4e6a8b0d1f3e5a7c"},            // gitleaks:allow -- public upstream regression fixture or detector example
		{`{"access_token": "0123456789abcdef0123"}`, "0123456789abcdef0123"}, // gitleaks:allow -- public upstream regression fixture or detector example
		{"DB_PASSWORD=0123456789ABCDEF01", "0123456789ABCDEF01"},             // gitleaks:allow -- public upstream regression fixture or detector example
		{"密钥为 5d41402abc4b2a76", "5d41402abc4b2a76"},
		{"apikey 为 5d41402abc4b2a77", "5d41402abc4b2a77"},
		// not a digest: mixed case, or letters past f
		{`{"key": "AbC123dEf456GhI789xyz"}`, "AbC123dEf456GhI789xyz"}, // gitleaks:allow -- public upstream regression fixture or detector example
	}
	for _, c := range masked {
		if out := Preview(c.s, o); strings.Contains(out, c.v) {
			t.Errorf("%q: key not masked: %q", c.s, out)
		}
	}

	// the model's SECRET on the same digits: kept under "key", a cache's
	// key and as a JSON key; masked in prose and under a credential's name
	m := &fakeModel{values: map[string]string{}}
	om := Options{Secrets: true, Personal: true, Model: m.find}
	for _, c := range []struct {
		s, v string
		gone bool
	}{
		{`{"key": "5d41402abc4b2a78"}`, "5d41402abc4b2a78", false},
		{`{"cacheKey": "5d41402abc4b2a79"}`, "5d41402abc4b2a79", false},
		{`{"keys": {"5d41402abc4b2a7a": 1}}`, "5d41402abc4b2a7a", false},
		{"the api key is 5d41402abc4b2a7b", "5d41402abc4b2a7b", true},
		{`{"secret": "5d41402abc4b2a7c"}`, "5d41402abc4b2a7c", true},
	} {
		m.values = map[string]string{c.v: "secret"}
		out := Preview(c.s, om)
		if gone := !strings.Contains(out, c.v); gone != c.gone {
			t.Errorf("%q, the model's secret: masked %v, want %v: %q", c.s, gone, c.gone, out)
		}
	}
}
