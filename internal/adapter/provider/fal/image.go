package fal

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// executeImage translates an OpenAI images request into a fal SYNC image call and
// the fal image response back into an OpenAI images response.
//
//	client  POST /v1/images/generations {model,prompt,n,size,response_format,...}
//	  ->    POST https://fal.run/{mappedModel} {prompt,image_size,...}
//	  <-    {"images":[{"url":...}], "seed":..., ...}
//	client  <- {"created":<unix>,"data":[{"url":...}]}   (or b64_json)
func (a *Adapter) executeImage(c *flow.Ctx) error {
	model := flow.GetMappedModel(c)
	if model == "" {
		model = gjson.GetBytes(flow.GetRequestBody(c), "model").String()
	}
	if strings.TrimSpace(model) == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "fal image request missing model")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	inBody := flow.GetRequestBody(c)
	responseFormat := strings.ToLower(strings.TrimSpace(gjson.GetBytes(inBody, "response_format").String()))

	falInput, err := buildFalImageInput(inBody)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to build fal image input")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	url := strings.TrimRight(resolveSyncBaseURL(), "/") + "/" + strings.TrimLeft(model, "/")
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	a.sendRequestInfo(c, http.MethodPost, url, falInput)
	respBody, _, err := a.doJSON(ctx, http.MethodPost, url, falInput)
	if err != nil {
		return err
	}

	openaiResp, err := a.buildOpenAIImageResponse(ctx, respBody, responseFormat)
	if err != nil {
		return err
	}
	return writeJSON(c, http.StatusOK, openaiResp)
}

// buildFalImageInput converts the OpenAI images body into fal's input JSON. The
// prompt passes through; sizing is translated (a client-supplied fal-native
// image_size wins, else the OpenAI "WxH" size becomes {width,height}); and every
// other client field is preserved so fal-native params (num_inference_steps,
// guidance_scale, seed, ...) are never dropped. OpenAI-only fields that fal does
// not understand (model, n, response_format, size) are stripped.
func buildFalImageInput(inBody []byte) ([]byte, error) {
	// Start from the client body so unknown/fal-native fields carry through.
	out := inBody
	if !gjson.ValidBytes(out) || strings.TrimSpace(string(out)) == "" {
		out = []byte("{}")
	}

	clientImageSize := gjson.GetBytes(out, "image_size")
	size := strings.TrimSpace(gjson.GetBytes(out, "size").String())

	var err error
	// Strip OpenAI-only fields fal doesn't consume.
	for _, f := range []string{"model", "n", "response_format", "size"} {
		out, err = sjson.DeleteBytes(out, f)
		if err != nil {
			return nil, err
		}
	}

	// Sizing: a fal-native image_size (string preset like "square"/"square_hd" or
	// object {width,height}) that the client already sent takes precedence. Only
	// when it's absent do we translate the OpenAI "WxH" size to fal's
	// {width,height} object.
	if !clientImageSize.Exists() && size != "" {
		if w, h, ok := parseWxH(size); ok {
			out, err = sjson.SetBytes(out, "image_size.width", w)
			if err != nil {
				return nil, err
			}
			out, err = sjson.SetBytes(out, "image_size.height", h)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// parseWxH parses an OpenAI-style "1024x1024" size into integer width/height.
func parseWxH(size string) (int, int, bool) {
	parts := strings.SplitN(strings.ToLower(size), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// buildOpenAIImageResponse translates fal's {"images":[{"url":...}]} into an
// OpenAI images response {"created":<unix>,"data":[...]}. For response_format
// "b64_json" it fetches each image's bytes from the fal URL and base64-encodes
// them; otherwise it returns the fal URL directly.
func (a *Adapter) buildOpenAIImageResponse(ctx context.Context, falBody []byte, responseFormat string) ([]byte, error) {
	images := gjson.GetBytes(falBody, "images")
	if !images.Exists() || len(images.Array()) == 0 {
		// Some fal models return a single "image" object instead of an "images"
		// array — accept both.
		if single := gjson.GetBytes(falBody, "image"); single.Exists() {
			images = gjson.Parse("[" + single.Raw + "]")
		}
	}

	wantB64 := responseFormat == "b64_json"
	out := []byte(`{"data":[]}`)
	var err error
	out, err = sjson.SetBytes(out, "created", time.Now().Unix())
	if err != nil {
		return nil, err
	}

	idx := 0
	for _, img := range images.Array() {
		url := img.Get("url").String()
		if url == "" {
			continue
		}
		if wantB64 {
			b64data, ferr := a.fetchAsBase64(ctx, url)
			if ferr != nil {
				return nil, ferr
			}
			out, err = sjson.SetBytes(out, "data."+strconv.Itoa(idx)+".b64_json", b64data)
		} else {
			out, err = sjson.SetBytes(out, "data."+strconv.Itoa(idx)+".url", url)
		}
		if err != nil {
			return nil, err
		}
		// Preserve fal's revised prompt hint if present (OpenAI compat field).
		if rp := gjson.GetBytes(falBody, "prompt").String(); rp != "" {
			out, _ = sjson.SetBytes(out, "data."+strconv.Itoa(idx)+".revised_prompt", rp)
		}
		idx++
	}
	return out, nil
}

// fetchAsBase64 downloads an image from a fal media URL (no auth needed) and
// returns its base64-encoded bytes for response_format=b64_json.
func (a *Adapter) fetchAsBase64(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to build fal image fetch")
		proxyErr.Scope = domain.ScopeRequest
		return "", proxyErr
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", domain.NewUpstreamConnectionError("failed to fetch fal image")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, isRetryableStatus(resp.StatusCode), "fal image fetch failed")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.HTTPStatusCode = resp.StatusCode
		return "", proxyErr
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", domain.NewUpstreamConnectionError("failed to read fal image bytes")
	}
	return b64Std(data), nil
}
