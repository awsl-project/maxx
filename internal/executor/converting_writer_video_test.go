package executor

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// A video request routed to a video-native provider must never be converted:
// there is no video converter, and the Gemini/Claude "preferred target"
// fallbacks must not hijack it.
func TestGetPreferredTargetTypeVideoStaysVideo(t *testing.T) {
	got := GetPreferredTargetType([]domain.ClientType{domain.ClientTypeVideo}, domain.ClientTypeVideo, "newapi")
	if got != domain.ClientTypeVideo {
		t.Fatalf("GetPreferredTargetType = %s, want %s", got, domain.ClientTypeVideo)
	}

	// Even if the provider happens to advertise other types alongside video, an
	// exact match on the original type wins first.
	got = GetPreferredTargetType(
		[]domain.ClientType{domain.ClientTypeOpenAI, domain.ClientTypeVideo},
		domain.ClientTypeVideo, "newapi")
	if got != domain.ClientTypeVideo {
		t.Fatalf("GetPreferredTargetType (mixed) = %s, want %s", got, domain.ClientTypeVideo)
	}
}

func TestNeedsConversionVideoToVideoIsFalse(t *testing.T) {
	if NeedsConversion(domain.ClientTypeVideo, domain.ClientTypeVideo) {
		t.Fatalf("NeedsConversion(video, video) = true, want false")
	}
}
