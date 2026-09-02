package fal

import (
	"context"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/adapter/client"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// new-api video status vocabulary (see seedance/code0 poll shape). fal statuses
// map onto these: IN_QUEUE/IN_PROGRESS -> IN_PROGRESS; COMPLETED -> SUCCESS.
const (
	statusInProgress = "IN_PROGRESS"
	statusSuccess    = "SUCCESS"
	statusFailed     = "FAILED"
)

// executeVideo dispatches the async video surface: POST = submit, GET = poll.
func (a *Adapter) executeVideo(c *flow.Ctx) error {
	uri := flow.GetRequestURI(c)
	if client.IsVideoPollPath(uri) {
		return a.executeVideoPoll(c, uri)
	}
	return a.executeVideoSubmit(c)
}

// executeVideoSubmit translates a new-api submit into a fal queue submit.
//
//	client  POST /v1/video/generations {model,prompt,...}
//	  ->    POST https://queue.fal.run/{mappedModel} {prompt,...}
//	  <-    {"request_id":"...","response_url":"https://queue.fal.run/{app}/requests/{id}"}
//	client  <- {"id":task,"task_id":task,"status":"queued"}
//
// task_id = base64url("{response_url}") so the poll is STATELESS — everything
// needed to reach the fal result/status URL is recovered from the id, and maxx
// stores nothing. We encode fal's returned response_url (not {model}) because fal
// strips a model's sub-route (e.g. fal-ai/veo3/fast -> fal-ai/veo3) in the queue
// request path; reconstructing from the mapped model would 404 on multi-segment
// ids.
func (a *Adapter) executeVideoSubmit(c *flow.Ctx) error {
	model := flow.GetMappedModel(c)
	if model == "" {
		model = gjson.GetBytes(flow.GetRequestBody(c), "model").String()
	}
	if strings.TrimSpace(model) == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "fal video submit missing model")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	falInput := buildFalVideoInput(flow.GetRequestBody(c))
	url := strings.TrimRight(resolveQueueBaseURL(), "/") + "/" + strings.TrimLeft(model, "/")
	ctx := reqCtx(c)

	a.sendRequestInfo(c, http.MethodPost, url, falInput)
	respBody, _, err := a.doJSON(ctx, http.MethodPost, url, falInput)
	if err != nil {
		return err
	}

	requestID := gjson.GetBytes(respBody, "request_id").String()
	responseURL := gjson.GetBytes(respBody, "response_url").String()
	if requestID == "" || responseURL == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "fal submit response missing request_id/response_url")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}

	taskID := encodeTaskID(responseURL)
	out := []byte(`{}`)
	out, _ = sjson.SetBytes(out, "id", taskID)
	out, _ = sjson.SetBytes(out, "task_id", taskID)
	out, _ = sjson.SetBytes(out, "status", "queued")
	return writeJSON(c, http.StatusOK, out)
}

// executeVideoPoll decodes the task_id back into fal's result URL, checks status,
// and — once COMPLETED — fetches the result and returns the new-api poll envelope.
//
//	client  GET /v1/video/generations/{task_id}
//	  ->    GET {responseURL}/status   (fal: IN_QUEUE|IN_PROGRESS|COMPLETED)
//	  ->    GET {responseURL}          (when COMPLETED: {"video":{"url":...}})
//	client  <- {"code":"success","data":{"status":"IN_PROGRESS|SUCCESS",...,"data":{...}}}
func (a *Adapter) executeVideoPoll(c *flow.Ctx, uri string) error {
	taskID := pollTaskID(uri)
	parts, err := decodeTaskID(taskID)
	if err != nil || len(parts) == 0 || parts[0] == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "invalid fal video task id")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}
	responseURL := parts[0]
	ctx := reqCtx(c)

	a.sendRequestInfo(c, http.MethodGet, responseURL+"/status", nil)
	statusBody, _, err := a.doJSON(ctx, http.MethodGet, responseURL+"/status", nil)
	if err != nil {
		return err
	}
	falStatus := strings.ToUpper(strings.TrimSpace(gjson.GetBytes(statusBody, "status").String()))

	switch falStatus {
	case "COMPLETED":
		a.sendRequestInfo(c, http.MethodGet, responseURL, nil)
		resultBody, _, err := a.doJSON(ctx, http.MethodGet, responseURL, nil)
		if err != nil {
			return err
		}
		return writeJSON(c, http.StatusOK, buildVideoPollEnvelope(taskID, statusSuccess, resultBody))
	case "IN_QUEUE", "IN_PROGRESS", "":
		return writeJSON(c, http.StatusOK, buildVideoPollEnvelope(taskID, statusInProgress, statusBody))
	default:
		// Any other terminal fal state (e.g. an error status) is surfaced as FAILED
		// so the polling client can stop.
		return writeJSON(c, http.StatusOK, buildVideoPollEnvelope(taskID, statusFailed, statusBody))
	}
}

// buildFalVideoInput strips new-api-only fields (model) and passes every other
// client field through to fal so fal-native params (duration, resolution,
// aspect_ratio, seed, image_url, ...) are preserved.
func buildFalVideoInput(inBody []byte) []byte {
	out := inBody
	if !gjson.ValidBytes(out) || strings.TrimSpace(string(out)) == "" {
		out = []byte("{}")
	}
	out, _ = sjson.DeleteBytes(out, "model")
	return out
}

// buildVideoPollEnvelope wraps a status + optional fal payload in the nested
// new-api poll shape clients expect: {"code":"success","data":{status, task_id,
// data:{...}}}. On SUCCESS it copies the fal result into data.data and lifts the
// resolved mp4 URL to data.data.url so simple clients can read it directly.
func buildVideoPollEnvelope(taskID, status string, falPayload []byte) []byte {
	out := []byte(`{"code":"success"}`)
	out, _ = sjson.SetBytes(out, "data.status", status)
	out, _ = sjson.SetBytes(out, "data.task_id", taskID)
	if len(falPayload) > 0 && gjson.ValidBytes(falPayload) {
		out, _ = sjson.SetRawBytes(out, "data.data", falPayload)
	}
	if status == statusSuccess {
		if url := extractVideoURL(falPayload); url != "" {
			out, _ = sjson.SetBytes(out, "data.data.url", url)
		}
	}
	return out
}

// extractVideoURL walks a fal video result for the resolved media URL. fal's
// common shape is {"video":{"url":...}}; some models nest it differently, so fall
// back to any top-level *.url string.
func extractVideoURL(body []byte) string {
	if u := gjson.GetBytes(body, "video.url").String(); u != "" {
		return u
	}
	if u := gjson.GetBytes(body, "video_url").String(); u != "" {
		return u
	}
	// Last resort: first *.url string field anywhere in the payload.
	var found string
	gjson.ParseBytes(body).ForEach(func(_, value gjson.Result) bool {
		if u := value.Get("url").String(); u != "" {
			found = u
			return false
		}
		return true
	})
	return found
}

// pollTaskID extracts the {task_id} segment from a video poll path. It must
// accept every prefix client.IsVideoPollPath routes here — the legacy
// /v1/video/generations/{task_id} and /video/generations/{task_id} forms as
// well as the /v1/videos/{task_id} and /videos/{task_id} forms added in #839.
// Missing the /videos variants made GET /videos/{id} return "invalid fal video
// task id" even though the gateway had already routed it to executeVideoPoll.
func pollTaskID(uri string) string {
	for _, base := range []string{
		"/v1/video/generations/",
		"/video/generations/",
		"/v1/videos/",
		"/videos/",
	} {
		if strings.HasPrefix(uri, base) {
			id := strings.TrimPrefix(uri, base)
			if i := strings.IndexAny(id, "/?"); i >= 0 {
				id = id[:i]
			}
			return id
		}
	}
	return ""
}

func reqCtx(c *flow.Ctx) context.Context {
	if c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}
