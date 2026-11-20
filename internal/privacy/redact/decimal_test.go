package redact

import (
	"strings"
	"testing"
)

// A decimal number is judged whole: the digits after its point, or before
// it, are not a phone, a card or an ID number of their own because they
// happen to check (willz on Discord: "height": 5.13393848347348).
func TestDecimalDigitsNotMasked(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, s := range []string{
		// the reporter's bytes, as a tool's output has them
		`{"height": 5.13393848347348}`,
		// a fraction a phone number's rule takes
		`{"lat": 0.13800138000, "lng": -3.13800138000}`,
		// a bank card's (Luhn checks), with an exponent after it
		`{"ratio": 6.4111111111111111e-3}`,
		// an ID card's
		`{"score": 1.110105194912310038}`,
		// the whole part, with a fraction after it
		`{"total": 13800138000.5}`,
		// in text, and many digits
		`the mean was 2.13800138000 over 12.41111111111111113333 runs`,
	} {
		out, n := Mask(s, o)
		if n != 0 || out != s {
			t.Errorf("Mask(%q) = %q (%d masked), want it as written", s, out, n)
		}
		body := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":` + jsonQuote(s) + `}]}]}`
		if got, n := MaskJSON([]byte(body), o); n != 0 || string(got) != body {
			t.Errorf("MaskJSON: %d masked: %s", n, got)
		}
	}
}

// What the local model takes for a phone or an account number inside a
// decimal is a piece of it too.
func TestDecimalDigitsNotMaskedByModel(t *testing.T) {
	const frac = "13393848347348"
	f := &fakeModel{values: map[string]string{frac: "account_number"}}
	o := Options{Secrets: true, Personal: true, Model: f.find}
	body := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"{\"height\": 5.` + frac + `}"}]}]}`
	if got, n := MaskJSON([]byte(body), o); n != 0 || string(got) != body {
		t.Fatalf("%d masked: %s", n, got)
	}
	// the same digits on their own are still the model's to mask
	body = `{"messages":[{"role":"user","content":"my account is ` + frac + `"}]}`
	if got, n := MaskJSON([]byte(body), o); n != 1 || strings.Contains(string(got), frac) {
		t.Fatalf("%d masked: %s", n, got)
	}
}

// A phone number masked before is not found again in a decimal's digits,
// in a thinking block the vendor signed either.
func TestKnownNumberNotInDecimal(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	const phone = "13800138000"
	if out, _ := Mask("call "+phone, o); strings.Contains(out, phone) {
		t.Fatalf("not masked to begin with: %q", out)
	}
	for _, s := range []string{"0." + phone, phone + ".25", "x=-7." + phone + "e2"} {
		if out, n := Mask("the value "+s+" is right", o); n != 0 {
			t.Errorf("%q: %d masked: %q", s, n, out)
		}
	}
	body := `{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"lat is 0.` + phone + `","signature":"Eqk1"}]}]}`
	if got, n := MaskJSON([]byte(body), o); n != 0 || string(got) != body {
		t.Fatalf("%d masked: %s", n, got)
	}
}

// Numbers that stand alone are still masked: on their own, in text,
// after a colon, at a sentence's end, beside a decimal that is not them.
func TestStandaloneNumbersStillMasked(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, c := range []struct{ in, kind, v string }{
		{"13800138000", "PHONE", "13800138000"},
		{"call 13800138000 now", "PHONE", "13800138000"},
		{"phone:13800138000", "PHONE", "13800138000"},
		{`{"phone": "13800138000"}`, "PHONE", "13800138000"},
		{"my number is 13800138000.", "PHONE", "13800138000"},
		{"价格 5.0，电话 13800138000", "PHONE", "13800138000"},
		{"v1.2 +86 13800138000", "PHONE", "13800138000"},
		{"card 4111 1111 1111 1111", "BANK_CARD", "4111 1111 1111 1111"},
		{"card 4111111111111111.", "BANK_CARD", "4111111111111111"},
		{"id 110105194912310038", "ID_CARD", "110105194912310038"},
	} {
		out, n := Mask(c.in, o)
		if n != 1 || strings.Contains(out, c.v) || !strings.Contains(out, "{{"+c.kind+"_") {
			t.Errorf("Mask(%q) = %q (%d masked), want %s masked as %s", c.in, out, n, c.v, c.kind)
		}
	}
	body := `{"messages":[{"role":"user","content":"{\"height\": 5.13393848347348, \"phone\": \"13800138000\"}"}]}`
	got, n := MaskJSON([]byte(body), o)
	if n != 1 || strings.Contains(string(got), "13800138000") || !strings.Contains(string(got), "5.13393848347348") {
		t.Fatalf("%d masked: %s", n, got)
	}
}

// A secret masked before is masked wherever it is, in a decimal's digits
// too: masking a number costs less than sending a password.
func TestKnownSecretInDecimalStillMasked(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	const secret = "4417hunter2Pass" // gitleaks:allow -- public upstream regression fixture or detector example
	if out, _ := Mask("DB_PASSWORD="+secret, o); strings.Contains(out, secret) {
		t.Fatalf("not masked to begin with: %q", out)
	}
	if out, n := Mask("the ratio is 0."+secret, o); n != 1 || strings.Contains(out, secret) {
		t.Fatalf("%d masked: %q", n, out)
	}
}

func jsonQuote(s string) string { return `"` + jsonEscape(s) + `"` }

// A number that stands bare as a JSON value is a count or a time, not a
// phone, a card or an ID number: a session file's line in a tool's output
// (id.null on Discord, "timestamp": 17929036837 masked as a PHONE). It is
// judged by where it sits, not by its key's name. The same digits in a
// JSON string, or in text, are still masked.
func TestBareJSONNumberNotMasked(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, s := range []string{
		// the reporter's line, as the session file has it
		`{"type":"user","timestamp": 17929036837,"uuid":"6316955a","isoTime":"2026-10-25T04:48:03.755Z"}`,
		// 10 and 13 digits, seconds and milliseconds
		`{"timestamp":1792903683,"id":"6316955a"}`,
		`{"timestamp":1792903683755}`,
		// 11 digits that start 1[3-9], at the object's end and spread out
		`{"n": 13800138000}`,
		"{\n  \"count\" :\t13800138000\n}",
		// in an array of numbers
		`{"ids":[13800138000, 13900139000]}`,
		`[1, 13800138000]`,
		// a Luhn-valid card's digits and an ID card's, as numbers
		`{"snowflake": 4111111111111111}`,
		`{"seq": 110105194912310038}`,
	} {
		out, n := Mask(s, o)
		if n != 0 || out != s {
			t.Errorf("Mask(%q) = %q (%d masked), want it as written", s, out, n)
		}
		body := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":` + jsonQuote(s) + `}]}]}`
		if got, n := MaskJSON([]byte(body), o); n != 0 || string(got) != body {
			t.Errorf("MaskJSON: %d masked: %s", n, got)
		}
	}
	for _, c := range []struct{ in, kind, v string }{
		{`{"phone": "13800138000"}`, "PHONE", "13800138000"},
		{`{"timestamp": 1792903683, "phone": "13800138000"}`, "PHONE", "13800138000"},
		{`["13800138000"]`, "PHONE", "13800138000"},
		{"phone:13800138000", "PHONE", "13800138000"},
		{"call 13800138000, thanks", "PHONE", "13800138000"},
		{"我的号码 13800138000，谢谢", "PHONE", "13800138000"},
		{"numbers 13900139000, 13800138000]", "PHONE", "13800138000"},
		{`{"note": "call 13800138000"}`, "PHONE", "13800138000"},
		{`{"card": "4111111111111111"}`, "BANK_CARD", "4111111111111111"},
	} {
		out, _ := Mask(c.in, o)
		if strings.Contains(out, c.v) || !strings.Contains(out, "{{"+c.kind+"_") {
			t.Errorf("Mask(%q) = %q, want %s masked as %s", c.in, out, c.v, c.kind)
		}
	}
}
