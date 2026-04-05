package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

// InviteCodeStatus represents the status of an invite code.
type InviteCodeStatus string

const (
	InviteCodeStatusActive   InviteCodeStatus = "active"
	InviteCodeStatusDisabled InviteCodeStatus = "disabled"
)

// InviteCodeInvalidPrefix is returned when a code cannot be normalized.
const InviteCodeInvalidPrefix = "<invalid-invite>"

// InviteCode represents an invitation code used for registration.
type InviteCode struct {
	ID        uint64     `json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	TenantID uint64 `json:"tenantID"`

	CodeHash   string           `json:"-"`
	CodePrefix string           `json:"codePrefix"`
	Status     InviteCodeStatus `json:"status"`

	MaxUses   uint64 `json:"maxUses"`
	UsedCount uint64 `json:"usedCount"`

	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	CreatedByUserID uint64 `json:"createdByUserID"`
	Note            string `json:"note,omitempty"`
}

// InviteCodeUsage records a usage event for an invite code.
type InviteCodeUsage struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	TenantID     uint64    `json:"tenantID"`
	InviteCodeID uint64    `json:"inviteCodeID"`
	UserID       uint64    `json:"userID"`
	Username     string    `json:"username"`
	UsedAt       time.Time `json:"usedAt"`

	IP        string `json:"ip"`
	UserAgent string `json:"userAgent"`

	Result     string `json:"result"`
	Reason     string `json:"reason,omitempty"`
	RolledBack bool   `json:"rolledBack,omitempty"`
}

// InviteCodeCreateItem contains a newly created invite code and its plain text.
type InviteCodeCreateItem struct {
	Code       string      `json:"code"`
	InviteCode *InviteCode `json:"inviteCode"`
}

// InviteCodeCreateResult is returned when creating invite codes.
type InviteCodeCreateResult struct {
	Items []InviteCodeCreateItem `json:"items"`
}

// NormalizeInviteCode trims and normalizes an invite code for hashing.
func NormalizeInviteCode(code string) string {
	var b strings.Builder
	b.Grow(len(code))
	for _, r := range code {
		if unicode.IsSpace(r) {
			continue
		}
		if isDashRune(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(b.String())
}

func isDashRune(r rune) bool {
	switch {
	case r == '-':
		return true
	case r >= 0x2010 && r <= 0x2015:
		return true
	case r == 0x2212, r == 0xFE58, r == 0xFE63, r == 0xFF0D:
		return true
	default:
		return false
	}
}

// HashInviteCode returns a SHA-256 hex hash for the given invite code.
func HashInviteCode(code string) string {
	normalized := NormalizeInviteCode(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// InviteCodePrefix returns a short prefix for display.
func InviteCodePrefix(code string) string {
	normalized := NormalizeInviteCode(code)
	if normalized == "" {
		return InviteCodeInvalidPrefix
	}
	if len(normalized) <= 8 {
		return normalized
	}
	return normalized[:8]
}
