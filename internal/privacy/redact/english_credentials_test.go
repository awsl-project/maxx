package redact

import (
	"strings"
	"testing"
)

func TestEnglishCredentialPresets(t *testing.T) {
	for _, tc := range []struct{ text, value string }{
		{"The password is OtherSyntheticSecret7865, keep it private.", "OtherSyntheticSecret7865"},
		{"My password was hunter2.", "hunter2"},
		{"The passphrase is \"Cobalt87!\".", "Cobalt87!"},
		{"PWD\tis\tCobalt87!", "Cobalt87!"},
		{"The API key is CobaltKey987654, keep it private.", "CobaltKey987654"},
		{"The api_key was 'CobaltKey987654'.", "CobaltKey987654"},
		{"Access token is CobaltAccess98765.", "CobaltAccess98765"},
		{"The refresh-token is CobaltRefresh98765", "CobaltRefresh98765"},
		{"clientSecret is CobaltClient98765", "CobaltClient98765"},
		{"Token is CobaltToken98765", "CobaltToken98765"},
		{"The password is " + strings.Repeat("Cobalt87", 50) + ".", strings.Repeat("Cobalt87", 50)},
	} {
		t.Run(tc.text, func(t *testing.T) {
			// Isolate cases: the known-value cache must not hide a missing rule.
			state := NewState()
			out, n := state.Mask(tc.text, Options{Secrets: true})
			if n != 1 || strings.Contains(out, tc.value) || !strings.Contains(out, "{{SECRET_") {
				t.Fatalf("preset did not mask the credential: %d %q", n, out)
			}
			if back := state.Restore(out, false); back != tc.text {
				t.Fatalf("roundtrip changed text: %q", back)
			}
		})
	}
}

func TestEnglishCredentialPresetsLeaveOrdinaryText(t *testing.T) {
	for _, text := range []string{
		"The password is invalid, please try again.",
		"The secret is confidential.",
		"The token is expired.",
		"The password is 12345678 characters long.",
		"The password is ${DB_PASSWORD}.",
		"The password is os.getenv(\"PASSWORD\").",
		"The token is config.token2.",
		"The password is getPassword256().",
		"The password is SHA256 hashed.",
		"The token is base64 encoded.",
		"The private key is /home/demo/keys/id_ed25519.",
		"The secret is env.SECRET_V2.",
		"The tokenizer is CobaltToken98765.",
		"The password isn't Cobalt87!",
	} {
		state := NewState()
		if out, n := state.Mask(text, Options{Secrets: true}); n != 0 || out != text {
			t.Errorf("ordinary text was masked: %q -> %q (%d)", text, out, n)
		}
	}
}
