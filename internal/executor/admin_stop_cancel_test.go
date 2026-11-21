package executor

import (
	"context"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestRequestFailureStatusAndErrorForAdminStoppedActiveRequest(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(domain.ErrActiveRequestStoppedByAdmin)

	status, message := requestFailureStatusAndError(ctx, context.Canceled)
	if status != "FAILED" {
		t.Fatalf("status = %q, want FAILED", status)
	}
	if message != domain.ActiveRequestStoppedByAdminMessage {
		t.Fatalf("message = %q, want %q", message, domain.ActiveRequestStoppedByAdminMessage)
	}
}
