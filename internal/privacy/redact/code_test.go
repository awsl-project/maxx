package redact

import (
	"strings"
	"testing"
)

// Code that names a secret is no secret: a TypeScript type, a field read,
// a call (Kieran Zhang on X, 10-25: a session's own source had each of
// these masked as SECRET). The lines are the reporter's, as the records
// view showed them.
func TestCodeIsNoSecret(t *testing.T) {
	for _, in := range []string{
		"...: string;   readonly signingKey: { readonly secretKey: Uint8Array; readonly publicKey: Uint8Array };   /** The host's cu...",
		"...     host_key: details.1,\n      private_key: details.2,              });         }     /// `#send` ...",
		"...     url,     hostKey: pair.hostKey,     privateKey: toBase64Url(device.privateKey),     account: { serverOrigin: clien...",
		`...ataDir, "relay-key");   try {     const secretKey = fromBase64Url((await readFile(path, "utf8")).trim());     if (secr...`,
		"type Keys = { apiKey: Base64Str | null }",
		"f({ token: opts.token2 })",
		"const password = sha256Hex (input)",
	} {
		if out, n := Mask(in, Options{Secrets: true}); n != 0 {
			t.Errorf("%q: %d masked: %q", in, n, out)
		}
	}
	// what a secret looks like written down stays masked: a value quoted,
	// one after =, one a line ends, as .env files, logs and YAML have them
	for in, v := range map[string]string{
		`secretKey: "Uint8Array9x",`:          "Uint8Array9x",
		"TOKEN=abc123def456":                  "abc123def456", // gitleaks:allow -- public upstream regression fixture or detector example
		"api_key=abc123def456, user=bob":      "abc123def456",
		"password: Passw0rd123\nuser: bob":    "Passw0rd123",
		"password: Passw0rd123":               "Passw0rd123",
		`{"secretKey": "abcd1234efgh"}`:       "abcd1234efgh", // gitleaks:allow -- public upstream regression fixture or detector example
		`{\"private_key\": \"abcd1234efgh\"}`: "abcd1234efgh",
	} {
		if out, n := Mask(in, Options{Secrets: true}); n != 1 || strings.Contains(unmarked(out), v) {
			t.Errorf("%q: %d masked: %q", in, n, out)
		}
	}
}
