package fal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
	// /v1/images/edits (image-to-image) is a distinct surface: its inbound body is
	// typically multipart/form-data carrying an uploaded image, and fal needs that
	// image supplied as an "image_url" (URL or data: URI). Route it separately.
	if isImageEditPath(flow.GetRequestURI(c)) {
		return a.executeImageEdit(c)
	}

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

	return a.postFalImage(c, model, falInput, responseFormat)
}

// isImageEditPath reports whether the inbound request targeted the OpenAI image
// EDIT surface (/v1/images/edits or /images/edits) rather than generations.
func isImageEditPath(uri string) bool {
	if i := strings.IndexAny(uri, "?#"); i >= 0 {
		uri = uri[:i]
	}
	uri = strings.TrimRight(uri, "/")
	return strings.HasSuffix(uri, "/images/edits")
}

// executeImageEdit translates an OpenAI images EDIT request (image-to-image) into
// a fal SYNC call. The inbound image is supplied to fal as an "image_url":
//
//   - multipart/form-data (the OpenAI default): the "image" file part is read and
//     base64-encoded into a data: URI; "prompt"/"size"/"mask"/etc. come from the
//     other form fields.
//   - JSON: some clients POST JSON with a client-provided "image_url" (a public
//     URL or data: URI) or "image"/"image_b64" (raw base64) — all accepted.
//
// The fal response ({"images":[{"url":...}]}) is translated back exactly like the
// generations path via buildOpenAIImageResponse.
func (a *Adapter) executeImageEdit(c *flow.Ctx) error {
	model := flow.GetMappedModel(c)

	var (
		falInput       []byte
		responseFormat string
		err            error
	)
	if isMultipartBody(c.Request) {
		falInput, responseFormat, model, err = buildFalImageEditFromMultipart(c.Request, flow.GetRequestBody(c), model)
	} else {
		falInput, responseFormat, model, err = buildFalImageEditFromJSON(flow.GetRequestBody(c), model)
	}
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to build fal image edit input")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	if strings.TrimSpace(model) == "" {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "fal image edit request missing model")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	return a.postFalImage(c, model, falInput, responseFormat)
}

// postFalImage POSTs a prepared fal image input to https://fal.run/{model} and
// writes the translated OpenAI images response. Shared by generations and edits.
func (a *Adapter) postFalImage(c *flow.Ctx, model string, falInput []byte, responseFormat string) error {
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

// isMultipartBody reports whether the request carries a multipart/form-data body.
func isMultipartBody(req *http.Request) bool {
	if req == nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Type")))
	return strings.HasPrefix(ct, "multipart/")
}

// buildFalImageEditFromMultipart reads an OpenAI images/edits multipart body and
// produces fal input JSON. The "image" file part becomes fal's "image_url" as a
// data: URI; "mask" (if present) becomes "mask_url"; text fields (prompt, size,
// n, strength, and any fal-native params) are folded in. It returns the fal input,
// the requested response_format, and the effective model (a "model" form field
// overrides the passed-in mapped model only when the latter is empty).
func buildFalImageEditFromMultipart(req *http.Request, body []byte, model string) ([]byte, string, string, error) {
	if req == nil {
		return nil, "", model, fmt.Errorf("nil request for multipart image edit")
	}
	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", model, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, "", model, fmt.Errorf("multipart image edit body without boundary")
	}

	out := []byte("{}")
	var responseFormat string
	var (
		imageData, maskData         []byte
		imageContentType, maskCType string
	)

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, perr := mr.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			return nil, "", model, perr
		}
		name := part.FormName()
		switch name {
		case "image":
			imageData, _ = io.ReadAll(part)
			imageContentType = part.Header.Get("Content-Type")
		case "mask":
			maskData, _ = io.ReadAll(part)
			maskCType = part.Header.Get("Content-Type")
		default:
			valBytes, _ := io.ReadAll(part)
			val := strings.TrimSpace(string(valBytes))
			_ = part.Close()
			switch name {
			case "model":
				if strings.TrimSpace(model) == "" {
					model = val
				}
			case "response_format":
				responseFormat = strings.ToLower(val)
			case "size":
				out, err = setImageSizeFromWxH(out, val)
			case "n":
				out, err = sjson.SetBytes(out, "num_images", jsonNumberOrString(val))
			default:
				// Preserve every other field (prompt, strength, guidance_scale,
				// num_inference_steps, seed, image_url, ...) as fal-native input.
				out, err = sjson.SetBytes(out, name, jsonNumberOrString(val))
			}
			if err != nil {
				return nil, "", model, err
			}
			continue
		}
		_ = part.Close()
	}

	if len(imageData) > 0 {
		out, err = sjson.SetBytes(out, "image_url", dataURI(imageContentType, imageData))
		if err != nil {
			return nil, "", model, err
		}
	}
	if len(maskData) > 0 {
		out, err = sjson.SetBytes(out, "mask_url", dataURI(maskCType, maskData))
		if err != nil {
			return nil, "", model, err
		}
	}
	return out, responseFormat, model, nil
}

// buildFalImageEditFromJSON handles a JSON-bodied images/edits request. The image
// may arrive as "image_url" (URL or data: URI), or as raw base64 under "image" or
// "image_b64" (wrapped into a data: URI). OpenAI-only fields are stripped and
// "size" is translated exactly like buildFalImageInput.
func buildFalImageEditFromJSON(inBody []byte, model string) ([]byte, string, string, error) {
	if !gjson.ValidBytes(inBody) || strings.TrimSpace(string(inBody)) == "" {
		return nil, "", model, fmt.Errorf("fal image edit: empty or invalid JSON body")
	}
	if strings.TrimSpace(model) == "" {
		if m := gjson.GetBytes(inBody, "model").String(); m != "" {
			model = m
		}
	}
	responseFormat := strings.ToLower(strings.TrimSpace(gjson.GetBytes(inBody, "response_format").String()))

	// Reuse the generations translation (strips model/n/response_format/size,
	// translates size→image_size, preserves fal-native params).
	out, err := buildFalImageInput(inBody)
	if err != nil {
		return nil, "", model, err
	}

	// Normalize the image source into fal's image_url. A client-supplied image_url
	// passes straight through; otherwise raw base64 under image/image_b64 becomes a
	// data: URI. "b64_json" here means response format, not an inline image, so it
	// is ignored as an image source.
	if !gjson.GetBytes(out, "image_url").Exists() {
		if b64 := firstNonEmpty(
			gjson.GetBytes(inBody, "image_b64").String(),
			gjson.GetBytes(inBody, "image").String(),
		); b64 != "" {
			src := b64
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), "data:") {
				src = dataURIFromBase64("", b64)
			}
			out, err = sjson.SetBytes(out, "image_url", src)
			if err != nil {
				return nil, "", model, err
			}
		}
	}
	// image/image_b64 are OpenAI-side field names fal doesn't consume.
	for _, f := range []string{"image", "image_b64"} {
		out, _ = sjson.DeleteBytes(out, f)
	}
	return out, responseFormat, model, nil
}

// setImageSizeFromWxH translates an OpenAI "WxH" size into fal's
// image_size{width,height}. A non-"WxH" value (e.g. a fal preset like "square_hd")
// is stored verbatim as image_size.
func setImageSizeFromWxH(body []byte, size string) ([]byte, error) {
	size = strings.TrimSpace(size)
	if size == "" {
		return body, nil
	}
	if w, h, ok := parseWxH(size); ok {
		var err error
		body, err = sjson.SetBytes(body, "image_size.width", w)
		if err != nil {
			return nil, err
		}
		return sjson.SetBytes(body, "image_size.height", h)
	}
	return sjson.SetBytes(body, "image_size", size)
}

// dataURI builds a data: URI from raw image bytes, defaulting an unknown media
// type to image/png (OpenAI edits accept png; fal probes the actual bytes anyway).
func dataURI(contentType string, data []byte) string {
	return dataURIFromBase64(contentType, b64Std(data))
}

func dataURIFromBase64(contentType, b64 string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "image/png"
	}
	return "data:" + contentType + ";base64," + b64
}

// jsonNumberOrString stores a form value as a JSON number when it parses cleanly
// (so fal receives strength=0.9 not "0.9"), else as a string.
func jsonNumberOrString(val string) interface{} {
	if val == "" {
		return val
	}
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	return val
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
