package handler

import (
	"errors"
	"net/mail"
	"strings"
)

var (
	errEmailRequired      = errors.New("email is required")
	errInvalidEmail       = errors.New("invalid email address")
	errDisposableEmail    = errors.New("disposable email addresses are not allowed")
	disposableEmailDomain = map[string]struct{}{
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
)

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
	for blocked := range disposableEmailDomain {
		if domain == blocked || strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
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
