package handler

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestValidateRegistrationEmail(t *testing.T) {
	origMX := emailLookupMX
	origHost := emailLookupHost
	defer func() {
		emailLookupMX = origMX
		emailLookupHost = origHost
	}()

	t.Run("valid email with mx record", func(t *testing.T) {
		emailLookupMX = func(_ context.Context, domain string) ([]*net.MX, error) {
			if domain == "example.com" {
				return []*net.MX{{Host: "mx.example.com.", Pref: 10}}, nil
			}
			return nil, errors.New("not found")
		}
		emailLookupHost = func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("unexpected host lookup")
		}

		email, err := validateRegistrationEmail(context.Background(), "User@Example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if email != "user@example.com" {
			t.Fatalf("expected normalized email user@example.com, got %s", email)
		}
	})

	t.Run("reject disposable domain", func(t *testing.T) {
		emailLookupMX = func(_ context.Context, _ string) ([]*net.MX, error) {
			return nil, errors.New("should not reach dns lookup")
		}
		emailLookupHost = func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("should not reach dns lookup")
		}

		_, err := validateRegistrationEmail(context.Background(), "user@mailinator.com")
		if err == nil || !strings.Contains(err.Error(), "disposable") {
			t.Fatalf("expected disposable email error, got %v", err)
		}
	})

	t.Run("fallback to host lookup when no mx", func(t *testing.T) {
		emailLookupMX = func(_ context.Context, _ string) ([]*net.MX, error) {
			return nil, errors.New("no mx")
		}
		emailLookupHost = func(_ context.Context, domain string) ([]string, error) {
			if domain == "example.org" {
				return []string{"127.0.0.1"}, nil
			}
			return nil, errors.New("not found")
		}

		email, err := validateRegistrationEmail(context.Background(), "user@example.org")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if email != "user@example.org" {
			t.Fatalf("expected normalized email user@example.org, got %s", email)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := validateRegistrationEmail(context.Background(), "not-an-email")
		if err == nil || !strings.Contains(err.Error(), "invalid email format") {
			t.Fatalf("expected invalid format error, got %v", err)
		}
	})

	t.Run("unresolvable domain", func(t *testing.T) {
		emailLookupMX = func(_ context.Context, _ string) ([]*net.MX, error) {
			return nil, errors.New("no mx")
		}
		emailLookupHost = func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("no host")
		}

		_, err := validateRegistrationEmail(context.Background(), "user@unresolvable.test")
		if err == nil || !strings.Contains(err.Error(), "not reachable") {
			t.Fatalf("expected unreachable domain error, got %v", err)
		}
	})

	t.Run("honors canceled parent context", func(t *testing.T) {
		mxSawCanceled := false
		hostSawCanceled := false
		emailLookupMX = func(ctx context.Context, _ string) ([]*net.MX, error) {
			mxSawCanceled = errors.Is(ctx.Err(), context.Canceled)
			return nil, ctx.Err()
		}
		emailLookupHost = func(ctx context.Context, _ string) ([]string, error) {
			hostSawCanceled = errors.Is(ctx.Err(), context.Canceled)
			return nil, ctx.Err()
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := validateRegistrationEmail(ctx, "user@example.com")
		if err == nil || !strings.Contains(err.Error(), "not reachable") {
			t.Fatalf("expected unreachable domain error, got %v", err)
		}
		if !mxSawCanceled || !hostSawCanceled {
			t.Fatalf("expected dns lookup context to be canceled, mx=%v host=%v", mxSawCanceled, hostSawCanceled)
		}
	})
}
