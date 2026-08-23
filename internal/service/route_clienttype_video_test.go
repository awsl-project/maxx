package service

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// A "video" route must be creatable via the admin API, so the client-type
// validation has to accept it alongside the chat surfaces.
func TestIsValidRouteClientTypeIncludesVideo(t *testing.T) {
	valid := []domain.ClientType{
		domain.ClientTypeClaude,
		domain.ClientTypeOpenAI,
		domain.ClientTypeCodex,
		domain.ClientTypeGemini,
		domain.ClientTypeVideo,
	}
	for _, ct := range valid {
		if !isValidRouteClientType(ct) {
			t.Fatalf("isValidRouteClientType(%s) = false, want true", ct)
		}
	}
	if isValidRouteClientType(domain.ClientType("bogus")) {
		t.Fatalf("isValidRouteClientType(bogus) = true, want false")
	}
}
