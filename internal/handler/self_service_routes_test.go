package handler

import "testing"

func TestSelfServiceRoutePatterns_IncludeTrailingSlashVariants(t *testing.T) {
	required := []string{
		"/api/provider-stats",
		"/api/provider-stats/",
		"/api/response-models",
		"/api/response-models/",
	}

	present := make(map[string]bool, len(selfServiceRoutePatterns))
	for _, pattern := range selfServiceRoutePatterns {
		present[pattern] = true
	}

	for _, pattern := range required {
		if !present[pattern] {
			t.Fatalf("missing self-service route pattern %q", pattern)
		}
	}
}
