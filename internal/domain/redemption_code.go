package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

// RedemptionCodeStatus represents a balance redemption code lifecycle state.
type RedemptionCodeStatus string

const (
	RedemptionCodeStatusActive   RedemptionCodeStatus = "active"
	RedemptionCodeStatusDisabled RedemptionCodeStatus = "disabled"
)

// RedemptionCode represents a one-time code that adds quota balance to a user-panel token.
type RedemptionCode struct {
	ID        uint64     `json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	TenantID uint64 `json:"tenantID"`

	CodeHash   string               `json:"-"`
	CodePrefix string               `json:"codePrefix"`
	Status     RedemptionCodeStatus `json:"status"`
	Amount     uint64               `json:"amount"`

	UsedByUserID uint64     `json:"usedByUserID,omitempty"`
	UsedAt       *time.Time `json:"usedAt,omitempty"`

	CreatedByUserID uint64 `json:"createdByUserID"`
	Note            string `json:"note,omitempty"`
}

// RedemptionCodeCreateItem contains a newly created redemption code and its plain text.
type RedemptionCodeCreateItem struct {
	Code           string          `json:"code"`
	RedemptionCode *RedemptionCode `json:"redemptionCode"`
}

// RedemptionCodeCreateResult is returned when creating redemption codes.
type RedemptionCodeCreateResult struct {
	Items []RedemptionCodeCreateItem `json:"items"`
}

// NormalizeRedemptionCode trims separators and normalizes a redemption code for hashing.
func NormalizeRedemptionCode(code string) string {
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range code {
		if unicode.IsSpace(r) || isDashRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}

// HashRedemptionCode returns a SHA-256 hex hash for the given redemption code.
func HashRedemptionCode(code string) string {
	normalized := NormalizeRedemptionCode(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// RedemptionCodePrefix returns a short prefix for display.
func RedemptionCodePrefix(code string) string {
	normalized := NormalizeRedemptionCode(code)
	if normalized == "" {
		return "<invalid-redemption>"
	}
	if len(normalized) <= 8 {
		return normalized
	}
	return normalized[:8]
}
