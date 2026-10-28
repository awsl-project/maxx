package redact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"regexp"
	"slices"
	"strings"
)

// A key can go to the vendor as a made-up key of its own form instead of
// a placeholder (Options.Shape): sk_w3axCByTQgK1Px… as sk_hYQWRBFmdHNI…,
// with the same prefix, length and separators and the same kind of
// character in each place. An agent's or a tool's check of a key's form
// passes, and the model has no placeholder to remark on (Charles Cao on X).
// It is made from the placeholder key as a placeholder is, so one key goes
// as the same made-up one every turn and a vendor's cache still matches,
// and it comes back as it was wherever the vendor writes it.
//
// Only a secret's kind, of minShape characters or more with minRandom of
// them made up (not its prefix or separators), and only letters, digits,
// '-', '_' and '.': a made-up key like that is in no other text by chance,
// and it needs no escaping in JSON. Anything else goes as a placeholder
// still. 16 left a short key such as sk_abcdefg a placeholder, which read
// as the setting not working.

const (
	minShape  = 8
	minRandom = 6
)

// keyPrefixes name a key's kind across more than one part, and are kept as
// they are; keyHead keeps one part of a few letters.
var keyPrefixes = []string{"sk-ant-api03-", "sk-ant-oat01-", "sk-ant-admin01-", "sk-proj-", "sk-svcacct-", "sk-or-v1-", "github_pat_"}

// keyHead is the start of v that names its kind and stays as it is: one of
// keyPrefixes, or up to four letters and the '-' or '_' after them (sk-,
// ghp_, xoxb-), or AWS's AKIA and ASIA. A head without its separator could
// be the key's own random letters, which stay with the user.
func keyHead(v string) string {
	for _, p := range keyPrefixes {
		if strings.HasPrefix(v, p) {
			return p
		}
	}
	if strings.HasPrefix(v, "AKIA") || strings.HasPrefix(v, "ASIA") {
		return v[:4]
	}
	for i := 0; i < len(v) && i <= 4; i++ {
		c := v[i]
		if (c == '-' || c == '_') && i > 0 {
			return v[:i+1]
		}
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			break
		}
	}
	return ""
}

// namedValue is a name before = or : and the value after it.
var namedValue = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*[ \t]*[=:][ \t]*()`)

// splitName parts a value found with its name, as 密钥
// OPENAI_API_KEY=sk_… is, into the name, which stays as written, and the
// key after it, which alone is made up and put back.
func splitName(v string) (name, key string) {
	if m := namedValue.FindStringSubmatchIndex(v); m != nil {
		return v[:m[2]], v[m[2]:]
	}
	return "", v
}

func shapeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
}

// shapeOf is the made-up key v of kind goes as, if it may go as one.
func (scopeState *State) shapeOf(kind, v string) (string, bool) {
	if _, personal := kindCats[kind]; personal || kind == "TERM" || len(v) < minShape {
		return "", false
	}
	for i := 0; i < len(v); i++ {
		if !shapeByte(v[i]) {
			return "", false
		}
	}
	head := keyHead(v)
	rest := v[len(head):]
	// a hex key's characters are any hex digit, of the case it has
	hexLower, hexUpper := true, true
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case c >= '0' && c <= '9', c == '-' || c == '_' || c == '.':
		case c >= 'a' && c <= 'f':
			hexUpper = false
		case c >= 'A' && c <= 'F':
			hexLower = false
		default:
			hexLower, hexUpper = false, false
		}
	}
	h := hmac.New(sha256.New, scopeState.key())
	h.Write([]byte("shape\x00" + kind + "\x00" + v))
	seed := h.Sum(nil)
	var stream []byte
	n := 0
	next := func(k int) byte {
		if len(stream) == 0 {
			h := hmac.New(sha256.New, seed)
			var c [8]byte
			binary.BigEndian.PutUint64(c[:], uint64(n))
			n++
			h.Write(c[:])
			stream = h.Sum(nil)
		}
		b := stream[0]
		stream = stream[1:]
		return b % byte(k)
	}
	const (
		upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lower = "abcdefghijklmnopqrstuvwxyz"
		digit = "0123456789"
	)
	out, made := []byte(head), 0
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		set := ""
		switch {
		case c == '-' || c == '_' || c == '.':
		case hexLower:
			set = digit + "abcdef"
		case hexUpper:
			set = digit + "ABCDEF"
		case c >= 'A' && c <= 'Z':
			set = upper
		case c >= 'a' && c <= 'z':
			set = lower
		default:
			set = digit
		}
		if set == "" {
			out = append(out, c)
			continue
		}
		made++
		out = append(out, set[next(len(set))])
	}
	if made < minRandom {
		return "", false
	}
	f := string(out)
	return f, f != v
}
func shapeOf(kind, v string) (string, bool) { return defaultState.shapeOf(kind, v) }

// shaped is put for Options.Shape: v as a made-up key of its form where it
// may go as one, else as its placeholder.
func (scopeState *State) shaped(kind, v string) string {
	name, k := splitName(v)
	f, ok := scopeState.shapeOf(kind, k)
	if !ok {
		return scopeState.placeholder(kind, v)
	}
	scopeState.checkValueSize(f, k)
	scopeState.mu.Lock()
	have, taken := scopeState.values[f]
	if !taken {
		scopeState.makeRoom()
		scopeState.storeValue(f, k)
		scopeState.kinds[kind] = true
		scopeState.shapes[f] = kind
		scopeState.addKnown(f, k)
	}
	scopeState.mu.Unlock()
	if taken && have != k {
		// another value's made-up key, which nothing ever is by chance
		return scopeState.placeholder(kind, v)
	}
	return name + f
}
func shaped(kind, v string) string { return defaultState.shaped(kind, v) }

// restoreShapes puts back each made-up key in s, counting it in put
// where put isn't nil; called with mu held for reading. A made-up key
// needs no escaping, nor does the value it stands for.
func (scopeState *State) restoreShapes(s string, put map[string]int) string {
	for f := range scopeState.shapes {
		if n := strings.Count(s, f); n > 0 {
			s = strings.ReplaceAll(s, f, scopeState.values[f])
			if put != nil {
				put[f] += n
			}
		}
	}
	return s
}
func restoreShapes(s string, put map[string]int) string { return defaultState.restoreShapes(s, put) }

// shapeTail is how much of the end of s may be the start of a made-up
// key, which the next piece of a stream goes on with: the longest end of s
// that one of them starts with, after the last whole one in s. A key that
// ends as one starts (…sqZs, sk_…) has come whole, and is put back whole.
func (scopeState *State) shapeTail(s string) int {
	scopeState.mu.RLock()
	defer scopeState.mu.RUnlock()
	whole := 0
	for f := range scopeState.shapes {
		if i := strings.LastIndex(s, f); i >= 0 {
			whole = max(whole, i+len(f))
		}
	}
	best := 0
	for f := range scopeState.shapes {
		for k := min(len(f)-1, len(s)-whole); k > best; k-- {
			if strings.HasSuffix(s, f[:k]) {
				best = k
				break
			}
		}
	}
	return best
}
func shapeTail(s string) int { return defaultState.shapeTail(s) }

// hasShapes says some made-up key is in b. With none made, as while
// Options.Shape is off, a whole reply isn't copied to find out.
func (scopeState *State) hasShapes(b []byte) bool {
	scopeState.mu.RLock()
	defer scopeState.mu.RUnlock()
	if len(scopeState.shapes) == 0 {
		return false
	}
	s := string(b)
	for f := range scopeState.shapes {
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}
func hasShapes(b []byte) bool { return defaultState.hasShapes(b) }

// outsideShapes is found without what lies in a made-up key already in s,
// as a placeholder's inside is left alone: a key that came back unrestored
// is not masked again as a key of its own.
func (scopeState *State) outsideShapes(s string, found []span) []span {
	scopeState.mu.RLock()
	defer scopeState.mu.RUnlock()
	for f := range scopeState.shapes {
		for i := 0; ; {
			j := strings.Index(s[i:], f)
			if j < 0 {
				break
			}
			a, z := i+j, i+j+len(f)
			found = slices.DeleteFunc(found, func(x span) bool { return x.start < z && x.end > a })
			i = z
		}
	}
	return found
}
func outsideShapes(s string, found []span) []span { return defaultState.outsideShapes(s, found) }

// A user name goes to the vendor as a made-up name, not a placeholder:
// C:\Users\eerttreeeert as C:\Users\qmzvkrta, never C:\Users\{{USERNAME_…}}.
// A path with braces in it reads as a template's or a temporary folder's,
// and an agent on mimo-v2.6-flash took the user's home for a temporary
// folder and set out to delete it (eerttreeeert on X, 10-28). A made-up
// name keeps the path a home folder's, which is what it is, and still
// tells the vendor nothing of the user. It is made from the name as a
// made-up key is (shapeOf), so it is the same every turn and a vendor's
// cache still matches, and it is put back wherever the vendor writes it
// (restoreShapes), in a tool call's path too. Only USERNAME's: what the
// user chose to mask (their words) goes as they chose.

// standInLen is a made-up name's length: eight lowercase letters, which
// read as a login name and are in no other text by chance.
const standInLen = 8

// standInOf is the made-up name the user name v goes as.
func (scopeState *State) standInOf(v string) string {
	h := hmac.New(sha256.New, scopeState.key())
	h.Write([]byte("user\x00" + v))
	sum := h.Sum(nil)
	out := make([]byte, standInLen)
	for i := range out {
		out[i] = 'a' + sum[i]%26
	}
	return string(out)
}
func standInOf(v string) string { return defaultState.standInOf(v) }

// standIn is put with a user name sent as its made-up name, which is
// kept to be put back as a made-up key is; put's own where that name is
// another value's already.
func (scopeState *State) standIn(put func(kind, v string) string) func(kind, v string) string {
	return func(kind, v string) string {
		if kind != "USERNAME" {
			return put(kind, v)
		}
		f := scopeState.standInOf(v)
		if f == v {
			return put(kind, v)
		}
		scopeState.checkValueSize(f, v)
		scopeState.mu.Lock()
		have, taken := scopeState.values[f]
		if !taken {
			scopeState.makeRoom()
			scopeState.storeValue(f, v)
			scopeState.kinds[kind] = true
			scopeState.shapes[f] = kind
			scopeState.addKnown(f, v)
		}
		scopeState.mu.Unlock()
		if taken && have != v {
			return put(kind, v)
		}
		return f
	}
}
func standIn(put func(kind, v string) string) func(kind, v string) string {
	return defaultState.standIn(put)
}
