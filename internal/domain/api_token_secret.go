package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	APITokenPrefix           = "maxx_"
	APITokenPrefixDisplayLen = 12
)

// HashAPIToken returns the stable SHA-256 digest used for stored API tokens.
func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NormalizeStoredAPIToken converts plaintext tokens into the stored digest form.
func NormalizeStoredAPIToken(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, APITokenPrefix) {
		return HashAPIToken(token)
	}
	return token
}
