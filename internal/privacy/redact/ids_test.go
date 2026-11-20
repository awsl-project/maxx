package redact

import (
	"strings"
	"testing"
)

// An id goes as written wherever it is and whatever is around it: a UUID,
// one whose last group holds a phone number's digits, one whose groups read
// as a card number, without hyphens, in capitals, a commit's hash. It is
// masked whole only in a field named for a credential.
func TestIDsGoAsWritten(t *testing.T) {
	forgetIDs := func() {
		defaultState.mu.Lock()
		defaultState.values = map[string]string{}
		reindexKnown()
		defaultState.mu.Unlock()
	}
	ids := []string{
		"3f2b8c1e-9a4d-4e7b-b2c1-7d6e5f4a3b21",
		"3F2B8C1E-9A4D-4E7B-B2C1-7D6E5F4A3B21",
		"3f2b8c1e-9a4d-4e7b-b2c1-a13800138000",         // PHONE in its last group
		"abcd4111-1111-1111-9007-abcdef123456",         // BANK_CARD across its groups
		"3f2b8c1e9a4d4e7bb2c1a13800138000",             // no hyphens
		"9f8e7d6cabcd4111111111111111900713800138000a", // a long hash with both
		"12345678-1234-1234-1234-123456789012",
	}
	kinds := map[string]bool{}
	for _, c := range Categories {
		kinds[c.ID] = true
	}
	o := Options{Secrets: true, Personal: true, Kinds: kinds}
	asWritten := []string{
		"%s", "id %s", `{"id":"%s"}`, "request_id: %s", "/users/%s/profile", "urn:uuid:%s", "{%s}",
		"next_page_token: %s", `"nextPageToken": "%s"`, `\"csrf_token\": \"%s\"`, `"token_id": "%s"`, "secret_id: %s",
		`"api_key_id": "%s"`, "access_key_id = %s", `"key": "%s"`, "Idempotency-Key: %s", "X-Request-Id: %s",
		"trace_token=%s", "secret_hash: %s", "sn: %s", "Serial Number: %s", "tel: %s",
	}
	for _, id := range ids {
		for _, c := range asWritten {
			forgetIDs()
			in := strings.ReplaceAll(c, "%s", id)
			if out, n := Mask(in, o); out != in || n != 0 {
				t.Errorf("%q went as %q", in, out)
			}
		}
	}
	credentials := []string{"password=%s", "api_key=%s", "X-Api-Key: %s", "token: %s", `"refresh_token": "%s"`, "session_token=%s", "postgres://app:%s@db:5432/x"}
	for _, id := range ids[:2] {
		for _, c := range credentials {
			forgetIDs()
			in := strings.ReplaceAll(c, "%s", id)
			if out, n := Mask(in, o); strings.Contains(out, id) || n != 1 {
				t.Errorf("a credential went out: %q as %q", in, out)
			}
		}
	}

	// the model takes an id for anything at all: it goes as written
	for label := range modelKinds {
		forgetIDs()
		id := ids[0]
		mo := o
		mo.Model = (&fakeModel{values: map[string]string{id: label}}).find
		body := `{"messages":[{"role":"user","content":"look at order ` + id + ` please"}]}`
		if got, n := MaskJSON([]byte(body), mo); string(got) != body || n != 0 {
			t.Errorf("the model's %s: %s", label, got)
		}
	}

	// a phone number masked before is not masked inside an id after
	forgetIDs()
	Mask("call 13800138000", o)
	if in := "order " + ids[2]; func() bool { out, _ := Mask(in, o); return out != in }() {
		t.Errorf("a known phone number was masked inside %q", ids[2])
	}

	// a phone number and a card number of their own are still masked
	for _, in := range []string{"call 13800138000", "card 4111-1111-1111-1111", "call +8613800138000"} {
		forgetIDs()
		if out, n := Mask(in, o); n != 1 || out == in {
			t.Errorf("%q went as %q", in, out)
		}
	}

	// what the user chose stays theirs: their words, their rules
	forgetIDs()
	uo := o
	uo.Words = []string{ids[0]}
	if out, n := Mask("order "+ids[0], uo); strings.Contains(out, ids[0]) || n != 1 {
		t.Errorf("the user's word went out: %q", out)
	}
	forgetIDs()
}

// cua-driver's element tokens go as written, in the window states it
// hands the model and in the clicks the model asks for, as #3782's
// request had them: JSON laid out and compact, in a shell command, in a
// log line, inside a JSON string, and whatever the local model takes one
// for. In a field named for a credential, or not shaped as one, a value is
// masked as before, and so is s and 8 digits with no index outside an
// element's field (a student's number); the user's own words still mask
// one.
func TestElementTokensGoAsWritten(t *testing.T) {
	forget := func() {
		defaultState.mu.Lock()
		defaultState.values = map[string]string{}
		reindexKnown()
		defaultState.mu.Unlock()
	}
	kinds := map[string]bool{}
	for _, c := range Categories {
		kinds[c.ID] = true
	}
	o := Options{Secrets: true, Personal: true, Kinds: kinds}
	tokens := []string{"s00000001", "s00000003:74", "s0000002a:14", "s00007a3f:42", "s00010001:3"}
	asWritten := []string{
		"\"element_index\": 0,\n\"element_token\": \"%s\",\n\"enabled\": true,",
		`"depth":2,"element_index":74,"element_token":"%s","enabled":true`,
		`"element_index":74,"element_token": "%s","enabled":true`,
		`cua-driver click '{"pid":844,"window_id":10725,"element_token": "%s","session":"run-1"}'`,
		"element_token=%s",
		`\"element_token\":\"%s\"`,
		`{"elementToken": "%s"}`,
	}
	for _, tok := range tokens {
		for _, c := range asWritten {
			forget()
			in := strings.ReplaceAll(c, "%s", tok)
			if out, n := Mask(in, o); out != in || n != 0 {
				t.Errorf("%q went as %q", in, out)
			}
		}
	}

	// a window's state and a click in one request, a password beside them
	forget()
	body := `{"messages":[{"role":"tool","tool_call_id":"c1","content":"{\"elements\":[{\"element_index\":14,\"element_token\":\"s0000002a:14\"}],\"env\":\"DB_PASSWORD=hunter2abc9\"}"},` +
		`{"role":"assistant","tool_calls":[{"id":"c2","type":"function","function":{"name":"click","arguments":"{\"pid\":844,\"element_token\":\"s0000002a:14\"}"}}]}]}`
	if got, n := MaskJSON([]byte(body), o); strings.Count(string(got), "s0000002a:14") != 2 || strings.Contains(string(got), "hunter2abc9") || n != 1 {
		t.Errorf("%d masked: %s", n, got)
	}

	// the local model's every label: as written, with its index in a text
	// and without one in an element's field
	for label := range modelKinds {
		for v, text := range map[string]string{
			"s0000002a:14": "click s0000002a:14 again",
			"s00000001":    `{\"element_index\": 0, \"element_token\": \"s00000001\"}`,
		} {
			forget()
			mo := o
			mo.Model = (&fakeModel{values: map[string]string{v: label}}).find
			body := `{"messages":[{"role":"user","content":"` + text + `"}]}`
			if got, n := MaskJSON([]byte(body), mo); string(got) != body || n != 0 {
				t.Errorf("the model's %s: %s", label, got)
			}
		}
	}
	// s and 8 digits with no index, outside an element's field: what the
	// model takes it for
	for _, label := range []string{"account_number", "secret"} {
		for _, text := range []string{"my student number is s12345678", `"student_no": "s12345678"`} {
			forget()
			mo := o
			mo.Model = (&fakeModel{values: map[string]string{"s12345678": label}}).find
			if out, n := Mask(text, mo); strings.Contains(out, "s12345678") || n != 1 {
				t.Errorf("the model's %s went out: %q as %q", label, text, out)
			}
		}
	}

	// a field named for a credential: masked
	for _, v := range []string{"s00000001", "s00000003:74"} {
		for _, c := range []string{"password=%s", "token: %s", `"refresh_token": "%s"`, `"api_key": "%s"`} {
			forget()
			in := strings.ReplaceAll(c, "%s", v)
			if out, n := Mask(in, o); strings.Contains(out, v[:9]) || n != 1 {
				t.Errorf("a credential went out: %q as %q", in, out)
			}
		}
	}
	// a value of another shape in an element's field: masked
	for _, v := range []string{"abc123def456", "s0000000g1", "S0000002A", "s000000012", "s00000001x", "s00000001:x9"} {
		forget()
		in := `"element_token": "` + v + `"`
		if out, n := Mask(in, o); out == in || n != 1 {
			t.Errorf("%q went as %q", in, out)
		}
	}
	// a field with "element" in its name that is no element_token is a
	// credential's: a UUID or the token's shape in it is masked
	for _, c := range []string{`"element_api_key": "%s"`, `"elementSecret": "%s"`, `"element_secret_token": "%s"`} {
		for _, v := range []string{"3f2504e0-4f89-11d3-9a0c-0305e82c3301", "s00000001"} {
			forget()
			in := strings.ReplaceAll(c, "%s", v)
			if out, n := Mask(in, o); strings.Contains(out, v) || n != 1 {
				t.Errorf("a credential went out: %q as %q", in, out)
			}
		}
	}
	// a token the model labelled in prose, masked there, still goes as
	// written in the element_token of the click after it
	forget()
	mo := o
	mo.Model = (&fakeModel{values: map[string]string{"s00000001": "secret"}}).find
	prose := `{"messages":[{"role":"assistant","content":"I will click s00000001 now"}]}`
	if got, n := MaskJSON([]byte(prose), mo); strings.Contains(string(got), "s00000001") || n != 1 {
		t.Fatalf("the model's label in prose: %s", got)
	}
	click := `{"messages":[{"role":"assistant","content":"I will click s00000001 now"},` +
		`{"role":"assistant","tool_calls":[{"id":"c2","type":"function","function":{"name":"click","arguments":"{\"element_token\":\"s00000001\"}"}}]}]}`
	if got, _ := MaskJSON([]byte(click), mo); !strings.Contains(string(got), `element_token\":\"s00000001`) {
		t.Errorf("a token known from prose was masked in the click: %s", got)
	}

	// the user's own words
	forget()
	uo := o
	uo.Words = []string{"s0000002a"}
	if out, n := Mask(`"element_token": "s0000002a:14"`, uo); strings.Contains(out, "s0000002a") || n != 1 {
		t.Errorf("the user's word went out: %q", out)
	}
	forget()
}
