package reqpolicy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// ValidEffort reports whether s is a settable policy effort: empty (unset) or one
// of the ordered tiers. "auto" is intentionally rejected as a policy value — it
// is a request-side deferral, not something an operator caps or defaults to.
func ValidEffort(s string) bool {
	if strings.TrimSpace(s) == "" {
		return true
	}
	_, ok := parseRank(s)
	return ok
}

// ValidatePolicy checks a policy's effort values are settable tiers.
func ValidatePolicy(p *domain.ReasoningPolicy) error {
	if p == nil {
		return nil
	}
	if !ValidEffort(p.MaxEffort) {
		return fmt.Errorf("%w: invalid maxEffort %q (want one of none|minimal|low|medium|high)", domain.ErrInvalidInput, p.MaxEffort)
	}
	if !ValidEffort(p.DefaultEffort) {
		return fmt.Errorf("%w: invalid defaultEffort %q (want one of none|minimal|low|medium|high)", domain.ErrInvalidInput, p.DefaultEffort)
	}
	return nil
}

// ParsePolicyJSON parses and validates a global reasoning-policy JSON object.
// Empty input yields (nil, nil) — no policy.
func ParsePolicyJSON(s string) (*domain.ReasoningPolicy, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var p domain.ReasoningPolicy
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("%w: invalid reasoning policy JSON: %v", domain.ErrInvalidInput, err)
	}
	if err := ValidatePolicy(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ValidatePolicyJSON validates a global reasoning-policy JSON string.
func ValidatePolicyJSON(s string) error {
	_, err := ParsePolicyJSON(s)
	return err
}
