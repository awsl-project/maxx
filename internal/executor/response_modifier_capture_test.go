package executor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor/responsemodifier"
)

func TestResponseCaptureRecordsUnmodifiedBodyBeforeModifier(t *testing.T) {
	recorder := httptest.NewRecorder()
	provider := &domain.Provider{
		Type: "claude",
		Config: &domain.ProviderConfig{Claude: &domain.ProviderConfigClaude{
			ResponseModelMapping: map[string]string{"upstream-model": "client-model"},
		}},
	}
	modifierWriter := responsemodifier.NewResponseModifierWriter(recorder, provider, domain.ClientTypeClaude, false)
	if modifierWriter == nil {
		t.Fatal("expected modifier writer")
	}

	responseCapture := NewResponseCapture(modifierWriter)
	upstreamBody := `{"model":"upstream-model","content":[{"type":"text","text":"ok"}]}`

	responseCapture.WriteHeader(http.StatusOK)
	if _, err := responseCapture.Write([]byte(upstreamBody)); err != nil {
		t.Fatalf("capture write failed: %v", err)
	}
	if got := responseCapture.Body(); got != upstreamBody {
		t.Fatalf("capture body = %s, want %s", got, upstreamBody)
	}
	if got := recorder.Body.String(); got != "" {
		t.Fatalf("client body should stay buffered before modifier finalize, got %s", got)
	}

	if err := modifierWriter.Finalize(); err != nil {
		t.Fatalf("modifier finalize failed: %v", err)
	}

	clientBody := recorder.Body.String()
	if !strings.Contains(clientBody, `"model":"client-model"`) {
		t.Fatalf("client body was not modified: %s", clientBody)
	}
	if strings.Contains(clientBody, `"model":"upstream-model"`) {
		t.Fatalf("client body still contains upstream model: %s", clientBody)
	}
	if got := responseCapture.Body(); got != upstreamBody {
		t.Fatalf("capture body changed after modifier finalize = %s, want %s", got, upstreamBody)
	}
}
