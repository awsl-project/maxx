package redact

import (
	"regexp"
	"strings"
	"unicode"
)

// What follows a secret's name and a colon or = is not always the secret.
// The SECRET rules take a name with password, secret, token or api_key in
// it, then : or =, then the value; a prompt and a session's code have that
// shape where nothing is secret:
//
//   - a place in a file: internal/auth/token.go:L643-669. names lines
//     643 to 669 of token.go (AlpsNowX on X, 10-28). It went to the model
//     as {{SECRET_…}}, and as a value masked once, everywhere it came
//     again (L643-669. alone in the next prompt): the agent could no longer
//     read the lines it was pointed at. The name is a file's, not a field's.
//   - a file's or a folder's path: PRIVATE_KEY_PATH=~/.ssh/id_ed25519,
//     "credentials": "/Users/…/legacy_credentials/x7k2/adc.json" name where
//     a secret is kept, and the agent needs the path as written to read it
//     (AkinoNya on X, 10-28: folders and skills with a random string in
//     their name were replaced).
//   - code naming a secret: access_token = get_access_token_v2,
//     max_tokens=max_tokens_v3, secret: env.SECRET_V2 (binary4cat on X, // gitleaks:allow -- public upstream regression fixture or detector example
//     10-28: function names were masked). A key's characters are random;
//     a name made of words, one of them token, secret or password, is a
//     function's or a variable's.
//
// A secret written down still masks: password=hunter2pass9,
// API_KEY=AbC123dEf456GhI789, token: t0k3nValue2, a password of one word
// and digits (Passw0rd123, password123).

// fieldAt is where a value the SECRET rules found can be a secret: not
// code (notCode), and not a place in a file, a path or a name in code.
func fieldAt(s string, a, b int) bool {
	if !notCode(s, a, b) {
		return false
	}
	v := s[a:b]
	if v[0] == '=' {
		return false // Go's := with no space after it: secretKey:=base64Encode(raw)
	}
	return !fileField(s, a) && !lineRef(v) && !pathValue(v) && !namesASecret(v) && !skillName(s, a, b)
}

// skillName says s[a:b] is the second half of plugin:skill, as Claude
// Code names a plugin's skill and command (design-tokens:generate-v2,
// secret-scan:audit-x7k2p9q1): words in lower case joined by hyphens on
// both sides of a colon with no space around it. A key has a capital or
// a long random run (notASlug).
func skillName(s string, a, b int) bool {
	if a < 2 || s[a-1] != ':' || !skillSlugRe.MatchString(s[a:b]) || notASlug(s[a:b]) {
		return false
	}
	i := a - 1
	for i > 0 && strings.IndexByte("abcdefghijklmnopqrstuvwxyz0123456789-", s[i-1]) >= 0 {
		i--
	}
	return i < a-1 && (i == 0 || strings.IndexByte(alnum+"_.", s[i-1]) < 0) && skillSlugRe.MatchString(s[i:a-1])
}

// skillSlugRe is words in lower case joined by hyphens: a plugin's, a skill's
// or a command's name.
var skillSlugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)+$`)

// lineRefRe is a line or a range of lines, as an editor or a review names
// them: L643, L643-669, L12-L40, 643-669, 643:12, with a sentence's
// stop after it.
var lineRefRe = regexp.MustCompile(`^L?\d+(?:[-–:]L?\d+)*[.,;:]?$`)

// lineRef says v is a line or a range of lines, never a secret.
func lineRef(v string) bool { return lineRefRe.MatchString(v) }

// fileField says the name before the value at s[a] is a file's, or a
// path's last part, not a field's: token.go:L643-669, src/secret_store.ts:…,
// skills/secret-scanner:…. It reads back over the separator (: or =, and
// the quotes and spaces around it) to the name.
func fileField(s string, a int) bool {
	i := a
	for i > 0 && strings.IndexByte(" \t\"'\\", s[i-1]) >= 0 {
		i--
	}
	if i == 0 || s[i-1] != ':' && s[i-1] != '=' {
		return false
	}
	i--
	for i > 0 && strings.IndexByte(" \t\"'\\", s[i-1]) >= 0 {
		i--
	}
	end := i
	for i > 0 && strings.IndexByte(alnum+"_.-", s[i-1]) >= 0 {
		i--
	}
	name := s[i:end]
	if name == "" {
		return false
	}
	if i > 0 && (s[i-1] == '/' || s[i-1] == '\\') {
		return true // a path's last part
	}
	dot := strings.LastIndexByte(name, '.')
	return dot > 0 && sourceExt[strings.ToLower(name[dot+1:])]
}

// sourceExt are the extensions of files an agent reads and names with a
// line after them: code, docs and configs. Not .env, .txt, .key or .pem,
// whose content is what a secret is kept in.
var sourceExt = map[string]bool{
	"go": true, "ts": true, "tsx": true, "mts": true, "cts": true, "js": true, "jsx": true, "mjs": true, "cjs": true,
	"py": true, "pyi": true, "rb": true, "rs": true, "java": true, "kt": true, "kts": true, "swift": true,
	"c": true, "h": true, "cc": true, "cpp": true, "cxx": true, "hpp": true, "hh": true, "m": true, "mm": true,
	"cs": true, "fs": true, "php": true, "scala": true, "dart": true, "lua": true, "zig": true, "nim": true,
	"ex": true, "exs": true, "erl": true, "hs": true, "ml": true, "clj": true, "r": true, "jl": true,
	"sh": true, "bash": true, "zsh": true, "fish": true, "ps1": true, "vue": true, "svelte": true, "astro": true,
	"md": true, "mdx": true, "rst": true, "sql": true, "proto": true, "graphql": true, "gradle": true,
	"html": true, "css": true, "scss": true, "less": true, "json": true, "jsonc": true, "yaml": true, "yml": true,
	"toml": true, "xml": true, "tf": true, "ipynb": true,
}

// pathRoots are the folders an absolute path starts at.
var pathRoots = []string{
	"Users", "home", "tmp", "private", "var", "etc", "run", "opt", "usr", "mnt", "Volumes", "root", "srv",
	"data", "workspace", "workspaces", "app", "Applications", "Library", "System", "proc", "dev", "sys", "nix",
}

// pathValue says v is a file's or a folder's path: one from home (~/),
// from here (./, ../), or from a root folder (/Users/…, /tmp/…), with no
// + or = in it as base64 has.
func pathValue(v string) bool {
	if strings.ContainsAny(v, "+=") {
		return false
	}
	for _, p := range []string{"~/", "./", "../"} {
		if strings.HasPrefix(v, p) && len(v) > len(p) {
			return true
		}
	}
	if !strings.HasPrefix(v, "/") {
		return false
	}
	first, rest, ok := strings.Cut(v[1:], "/")
	if !ok || rest == "" {
		return false
	}
	for _, r := range pathRoots {
		if first == r {
			return true
		}
	}
	return false
}

// secretWords are the words of a name in code that say it holds a secret.
var secretWords = map[string]bool{
	"token": true, "tokens": true, "secret": true, "secrets": true, "password": true, "passwords": true,
	"passwd": true, "pwd": true, "key": true, "keys": true, "apikey": true, "credential": true, "credentials": true,
}

// identRe is a name in code, read through others or not: camelCase,
// snake_case, SCREAMING_CASE, env.SECRET_V2.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// namesASecret says v is a name in code for a secret, not one: two words
// or more, one of them token, secret, password or key, written as code
// writes a name: starting in lower case, or with _ or . between its
// words. refreshToken2, generate_token_v2, env.SECRET_V2 and // gitleaks:allow -- public upstream regression fixture or detector example
// countTokens4o are names; Passw0rd123 and password123 are one word, and
// SuperSecret2024 is a password, not how code names a value.
func namesASecret(v string) bool {
	if !identRe.MatchString(v) {
		return false
	}
	if !unicode.IsLower(rune(v[0])) && !strings.ContainsAny(v, "_.") {
		return false
	}
	words, named := 0, false
	for _, w := range identWords(v) {
		if strings.IndexFunc(w, unicode.IsLetter) < 0 {
			continue
		}
		words++
		if i := strings.IndexAny(w, digits); i >= 0 {
			w = w[:i] // countTokens4o: Tokens
		}
		if secretWords[strings.ToLower(w)] {
			named = true
		}
	}
	return words >= 2 && named
}

// identWords are v's words: split at _ and ., and where a capital starts
// a word in camelCase (refreshToken2 is refresh and Token2).
func identWords(v string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == '_' || r == '.' }) {
		start := 0
		for i := 1; i < len(part); i++ {
			if part[i] >= 'A' && part[i] <= 'Z' && part[i-1] >= 'a' && part[i-1] <= 'z' {
				out = append(out, part[start:i])
				start = i
			}
		}
		out = append(out, part[start:])
	}
	return out
}
