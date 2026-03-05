package handler

import (
	"context"
	"errors"
	"net"
	"net/mail"
	"strings"
	"time"
)

var (
	emailLookupMX = func(ctx context.Context, domain string) ([]*net.MX, error) {
		return net.DefaultResolver.LookupMX(ctx, domain)
	}
	emailLookupHost = func(ctx context.Context, host string) ([]string, error) {
		return net.DefaultResolver.LookupHost(ctx, host)
	}
)

var disposableEmailDomains = map[string]struct{}{
	"10minutemail.com":  {},
	"33mail.com":        {},
	"anonbox.net":       {},
	"dispostable.com":   {},
	"emailondeck.com":   {},
	"fakeinbox.com":     {},
	"getairmail.com":    {},
	"guerrillamail.com": {},
	"inboxbear.com":     {},
	"maildrop.cc":       {},
	"mailinator.com":    {},
	"moakt.com":         {},
	"mytemp.email":      {},
	"sharklasers.com":   {},
	"temp-mail.org":     {},
	"tempail.com":       {},
	"tempmail.dev":      {},
	"tempmailo.com":     {},
	"throwawaymail.com": {},
	"yopmail.com":       {},
}

func validateRegistrationEmail(rawEmail string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(rawEmail))
	if email == "" {
		return "", errors.New("email is required")
	}

	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", errors.New("invalid email format")
	}

	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", errors.New("invalid email format")
	}
	domain := email[at+1:]
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || !strings.Contains(domain, ".") {
		return "", errors.New("invalid email format")
	}

	if isDisposableDomain(domain) {
		return "", errors.New("anonymous or disposable email domains are not allowed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if mxRecords, mxErr := emailLookupMX(ctx, domain); mxErr == nil && len(mxRecords) > 0 {
		return email, nil
	}
	if hosts, hostErr := emailLookupHost(ctx, domain); hostErr == nil && len(hosts) > 0 {
		return email, nil
	}

	return "", errors.New("email domain is not reachable")
}

func isDisposableDomain(domain string) bool {
	if _, exists := disposableEmailDomains[domain]; exists {
		return true
	}
	for blocked := range disposableEmailDomains {
		if strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
}
