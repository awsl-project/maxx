package core

import (
	"fmt"
	"net"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

const loopbackHost = "127.0.0.1"

// LANAccessEnabled returns the effective LAN service setting.
// Missing/empty values default to true to preserve Maxx's historical :9880 bind.
func LANAccessEnabled(settings repository.SystemSettingRepository) bool {
	if settings == nil {
		return true
	}
	value, err := settings.Get(domain.SettingKeyLANAccessEnabled)
	if err != nil {
		return true
	}
	return strings.TrimSpace(strings.ToLower(value)) != "false"
}

// ResolveServerBindAddress applies the local-service LAN switch to the default
// server address. Explicit addresses are advanced overrides and are left intact.
func ResolveServerBindAddress(addr string, settings repository.SystemSettingRepository, explicitAddr bool) string {
	if explicitAddr || LANAccessEnabled(settings) {
		return addr
	}
	return loopbackBindAddress(addr)
}

func loopbackBindAddress(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return net.JoinHostPort(loopbackHost, "9880")
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err == nil {
		if port == "" {
			return trimmed
		}
		if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
			return net.JoinHostPort(loopbackHost, port)
		}
		return trimmed
	}

	if strings.HasPrefix(trimmed, ":") && len(trimmed) > 1 {
		return loopbackHost + trimmed
	}
	if allDigits(trimmed) {
		return net.JoinHostPort(loopbackHost, trimmed)
	}

	return fmt.Sprintf("%s", trimmed)
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
