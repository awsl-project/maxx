package handler

import (
	"errors"
	"fmt"
	"log"
	"net/mail"
	"os"
	"strings"
	"sync"
)

const disposableEmailDomainsFileEnvKey = "MAXX_DISPOSABLE_EMAIL_DOMAINS_FILE"

var (
	errEmailRequired   = errors.New("email is required")
	errInvalidEmail    = errors.New("invalid email address")
	errDisposableEmail = errors.New("disposable email addresses are not allowed")

	defaultDisposableEmailDomains = map[string]struct{}{
		"10minutemail.com":  {},
		"dispostable.com":   {},
		"fakeinbox.com":     {},
		"getairmail.com":    {},
		"getnada.com":       {},
		"guerrillamail.com": {},
		"maildrop.cc":       {},
		"mailinator.com":    {},
		"mailnesia.com":     {},
		"moakt.com":         {},
		"sharklasers.com":   {},
		"tempmail.dev":      {},
		"tempmail.email":    {},
		"tempmailo.com":     {},
		"temp-mail.org":     {},
		"tempmail.plus":     {},
		"tmpmail.org":       {},
		"trashmail.com":     {},
		"yopmail.com":       {},
	}

	disposableEmailDomainsMu sync.RWMutex
	disposableEmailDomains   = cloneDomainSet(defaultDisposableEmailDomains)
)

func init() {
	if err := loadDisposableDomains(); err != nil {
		log.Printf("[Auth] failed to load disposable email domains from %s: %v; using bundled defaults", disposableEmailDomainsFileEnvKey, err)
	}
}

// ReloadDisposableDomains reloads disposable email domains from configured source.
// When loading fails, the bundled defaults remain active.
func ReloadDisposableDomains() error {
	return loadDisposableDomains()
}

// NOTE: This validation only checks syntax and disposable-domain policy.
// TODO: Add ownership verification (OTP/activation email) to prove mailbox control.
func normalizeAndValidateRegistrationEmail(raw string) (string, error) {
	email := strings.TrimSpace(strings.ToLower(raw))
	if email == "" {
		return "", errEmailRequired
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", errInvalidEmail
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "", errInvalidEmail
	}
	domain := parts[1]
	if !isLikelyValidDomain(domain) {
		return "", errInvalidEmail
	}

	if isDisposableEmailDomain(domain) {
		return "", errDisposableEmail
	}
	return email, nil
}

func isDisposableEmailDomain(domain string) bool {
	normalized, ok := normalizeDomain(domain)
	if !ok {
		return false
	}

	disposableEmailDomainsMu.RLock()
	defer disposableEmailDomainsMu.RUnlock()

	for current := normalized; current != ""; {
		if _, exists := disposableEmailDomains[current]; exists {
			return true
		}

		nextDot := strings.IndexByte(current, '.')
		if nextDot < 0 {
			break
		}
		current = current[nextDot+1:]
	}
	return false
}

func loadDisposableDomains() error {
	domains := cloneDomainSet(defaultDisposableEmailDomains)

	path := strings.TrimSpace(os.Getenv(disposableEmailDomainsFileEnvKey))
	if path == "" {
		setDisposableEmailDomains(domains)
		return nil
	}

	loadedDomains, err := loadDisposableDomainsFromFile(path)
	if err != nil {
		setDisposableEmailDomains(domains)
		return err
	}

	for domain := range loadedDomains {
		domains[domain] = struct{}{}
	}
	setDisposableEmailDomains(domains)
	return nil
}

func loadDisposableDomainsFromFile(path string) (map[string]struct{}, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	domains := make(map[string]struct{})
	lines := strings.Split(string(content), "\n")
	for idx, line := range lines {
		item := strings.TrimSpace(line)
		if item == "" || strings.HasPrefix(item, "#") {
			continue
		}

		domain, ok := normalizeDomain(item)
		if !ok {
			return nil, fmt.Errorf("invalid domain %q at line %d", item, idx+1)
		}
		domains[domain] = struct{}{}
	}

	if len(domains) == 0 {
		return nil, errors.New("domain list is empty")
	}

	return domains, nil
}

func setDisposableEmailDomains(domains map[string]struct{}) {
	disposableEmailDomainsMu.Lock()
	disposableEmailDomains = domains
	disposableEmailDomainsMu.Unlock()
}

func cloneDomainSet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for domain := range src {
		dst[domain] = struct{}{}
	}
	return dst
}

func normalizeDomain(domain string) (string, bool) {
	normalized := strings.TrimSpace(strings.ToLower(domain))
	normalized = strings.TrimPrefix(normalized, ".")
	if !isLikelyValidDomain(normalized) {
		return "", false
	}
	return normalized, true
}

func isLikelyValidDomain(domain string) bool {
	if domain == "" || strings.Contains(domain, "..") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}
