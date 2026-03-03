package handler

import (
	"net/http"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

// memberAllowedResources defines resources that member role can access (GET only)
var memberAllowedResources = map[string]bool{
	"dashboard":     true,
	"requests":      true,
	"sessions":      true,
	"usage-stats":   true,
	"proxy-status":  true,
	"cooldowns":     true,
	"logs":          true,
}

// CheckRBAC checks if the current user has permission for the given resource and method
// Returns true if access is allowed
func CheckRBAC(r *http.Request, resource string) bool {
	role := maxxctx.GetUserRole(r.Context())

	// Empty role means no auth context — deny by default
	if role == "" {
		return false
	}

	// Admin has full access
	if role == string(domain.UserRoleAdmin) {
		return true
	}

	// Member can only GET certain resources
	if role == string(domain.UserRoleMember) {
		if r.Method != http.MethodGet {
			return false
		}
		return memberAllowedResources[resource]
	}

	return false
}
