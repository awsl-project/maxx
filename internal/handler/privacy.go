package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy"
	"github.com/awsl-project/maxx/internal/privacy/pii"
	"github.com/awsl-project/maxx/internal/privacy/redact"
	"github.com/awsl-project/maxx/internal/service"
)

// Both existing project API routes delegate to this independent module handler.
// The enclosing authentication middleware supplies the tenant and admin role.
func handleProjectPrivacy(w http.ResponseWriter, r *http.Request, svc *service.AdminService, id uint64) {
	w.Header().Set("Cache-Control", "no-store")
	if !CheckRBAC(r, "projects") {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	tenant := maxxctx.GetTenantID(r.Context())
	if id == 0 {
		writeJSON(w, 400, map[string]string{"error": "project id required"})
		return
	}
	p := svc.Privacy()
	cfg, err := p.Configuration(tenant, id)
	if err != nil {
		privacyError(w, err)
		return
	}
	suffix := ""
	if _, tail, ok := strings.Cut(r.URL.Path, "/privacy"); ok {
		suffix = strings.Trim(tail, "/")
	}
	parts := strings.Split(suffix, "/")
	switch parts[0] {
	case "":
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, cfg)
		case http.MethodPut:
			var input struct {
				BlockSecrets *bool           `json:"blockSecrets"`
				Config       *privacy.Config `json:"config"`
			}
			if !privacyDecode(w, r, &input, 256<<10) {
				return
			}
			if (input.BlockSecrets == nil) == (input.Config == nil) {
				writeJSON(w, 400, map[string]string{"error": "provide config or blockSecrets"})
				return
			}
			var project *domain.Project
			if input.Config != nil {
				valid, validationErr := privacy.ValidateConfig(*input.Config)
				if validationErr != nil {
					writeJSON(w, 400, map[string]string{"error": validationErr.Error()})
					return
				}
				for _, providerID := range valid.BypassProviders {
					if _, err := svc.GetProvider(tenant, providerID); err != nil {
						writeJSON(w, 400, map[string]string{"error": "invalid privacy provider exception"})
						return
					}
				}
				project, err = p.SetConfiguration(tenant, id, valid)
			} else {
				project, err = svc.UpdateProjectPrivacy(tenant, id, *input.BlockSecrets)
			}
			if err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, project)
		default:
			privacyMethod(w, "GET, PUT")
		}
	case "categories":
		if r.Method != http.MethodGet {
			privacyMethod(w, "GET")
			return
		}
		writeJSON(w, 200, redact.Categories)
	case "preview":
		if r.Method != http.MethodPost {
			privacyMethod(w, "POST")
			return
		}
		var input struct {
			Config privacy.Config `json:"config"`
			Text   *string        `json:"text"`
			Body   *string        `json:"body"`
		}
		if !privacyDecode(w, r, &input, 512<<10) {
			return
		}
		if (input.Text == nil) == (input.Body == nil) {
			writeJSON(w, 400, map[string]string{"error": "provide text or JSON body"})
			return
		}
		var body []byte
		if input.Text != nil {
			body = privacy.TextRequest(*input.Text)
		} else {
			body = []byte(*input.Body)
		}
		result, rejection := p.Preview(r.Context(), input.Config, body)
		if rejection != nil {
			writeJSON(w, rejection.HTTPStatusCode, map[string]string{"error": rejection.Message, "code": rejection.Code})
			return
		}
		writeJSON(w, 200, result)
	case "models":
		if r.Method == http.MethodGet && len(parts) == 1 {
			statuses := []pii.Status{}
			for _, model := range pii.Models() {
				statuses = append(statuses, model.Now())
			}
			writeJSON(w, 200, map[string]any{"items": statuses, "canManage": tenant == domain.DefaultTenantID})
			return
		}
		if r.Method != http.MethodPost {
			privacyMethod(w, "GET, POST")
			return
		}
		// Weights/processes are shared public resources. Their lifecycle belongs to
		// the server's default-tenant administrators, not another tenant's project.
		if tenant != domain.DefaultTenantID {
			writeJSON(w, 403, map[string]string{"error": "shared models are managed by the server administrator"})
			return
		}
		if len(parts) != 3 || parts[1] != pii.PrivacyFilter && parts[1] != pii.AstrLinkGuard {
			writeJSON(w, 404, map[string]string{"error": "unknown privacy model action"})
			return
		}
		model := pii.Of(parts[1])
		var actionErr error
		switch parts[2] {
		case "download":
			actionErr = model.Download(nil)
		case "cancel":
			model.Cancel()
		case "warm":
			if !model.Installed() {
				writeJSON(w, 409, map[string]string{"error": "download the model first"})
				return
			}
			model.Warm()
		case "stop":
			model.Stop()
		case "remove":
			actionErr = model.Remove()
		default:
			writeJSON(w, 404, map[string]string{"error": "unknown privacy model action"})
			return
		}
		if actionErr != nil {
			writeJSON(w, 503, map[string]string{"error": "local model action failed"})
			return
		}
		writeJSON(w, 200, model.Now())
	case "summary":
		if r.Method != http.MethodGet {
			privacyMethod(w, "GET")
			return
		}
		result, err := p.Summary(tenant, id)
		if err != nil {
			privacyError(w, err)
			return
		}
		writeJSON(w, 200, result)
	case "records":
		switch {
		case len(parts) == 1 && r.Method == http.MethodDelete:
			if err := p.ClearRecords(tenant, id); err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, map[string]bool{"cleared": true})
		case len(parts) == 1 && r.Method == http.MethodGet:
			q := r.URL.Query()
			page, _ := strconv.Atoi(q.Get("page"))
			limit, _ := strconv.Atoi(q.Get("limit"))
			provider, _ := strconv.ParseUint(q.Get("providerId"), 10, 64)
			since, _ := time.Parse(time.RFC3339, q.Get("since"))
			until, _ := time.Parse(time.RFC3339, q.Get("until"))
			result, err := p.Records(tenant, id, privacy.RecordsQuery{Page: page, Limit: limit, Kind: q.Get("kind"), Action: q.Get("action"), By: q.Get("by"), ProviderID: provider, Since: since, Until: until})
			if err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, result)
		case len(parts) == 2 && r.Method == http.MethodGet:
			result, err := p.Record(tenant, id, parts[1])
			if err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, result)
		case len(parts) == 3 && parts[2] == "value" && r.Method == http.MethodGet:
			result, err := p.Original(tenant, id, parts[1], r.URL.Query().Get("placeholder"))
			if err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, map[string]string{"value": result})
		case len(parts) == 3 && parts[2] == "sent" && r.Method == http.MethodGet:
			result, err := p.Sent(tenant, id, parts[1])
			if err != nil {
				privacyError(w, err)
				return
			}
			writeJSON(w, 200, map[string]string{"body": result})
		default:
			writeJSON(w, 404, map[string]string{"error": "not found"})
		}
	default:
		writeJSON(w, 404, map[string]string{"error": "not found"})
	}
}
func privacyDecode(w http.ResponseWriter, r *http.Request, out any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid privacy settings"})
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid privacy settings"})
		return false
	}
	return true
}
func privacyError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, 404, map[string]string{"error": "project or privacy record not found"})
		return
	}
	writeJSON(w, 500, map[string]string{"error": "privacy operation failed"})
}
func privacyMethod(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}
