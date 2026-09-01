package fal

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
)

const testKey = "id-part:secret-part"

func newFalAdapter(t *testing.T) *Adapter {
	t.Helper()
	a, err := NewAdapter(&domain.Provider{
		Name: "fal-test",
		Type: "fal",
		Config: &domain.ProviderConfig{
			Fal: &domain.ProviderConfigFal{APIKey: testKey},
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return a.(*Adapter)
}

// newVideoCtx wires a flow.Ctx that mimics the proxy: it stashes the client-facing
// auth headers (which must be stripped) plus the URI/body/client-type keys.
func newCtx(method, uri string, body []byte, ct domain.ClientType) (*flow.Ctx, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "http://localhost"+uri, nil)
	req.Header.Set("Authorization", "Bearer sk-maxx-client-token")
	req.Header.Set("x-api-key", "sk-maxx-client-token")
	req.Header.Set("x-goog-api-key", "sk-maxx-client-token")
	req.Header.Set("Proxy-Authorization", "Bearer sk-maxx-client-token")
	rec := httptest.NewRecorder()
	c := flow.NewCtx(rec, req)
	c.Set(flow.KeyClientType, ct)
	c.Set(flow.KeyRequestURI, uri)
	c.Set(flow.KeyRequestBody, body)
	c.Set(flow.KeyRequestHeaders, req.Header)
	return c, rec
}

func TestNewAdapterRequiresFalConfig(t *testing.T) {
	for _, p := range []*domain.Provider{
		{Name: "f", Config: nil},
		{Name: "f", Config: &domain.ProviderConfig{}},
	} {
		if _, err := NewAdapter(p); err == nil {
			t.Fatalf("expected error for missing fal config")
		}
	}
}

func TestSupportedClientTypes(t *testing.T) {
	a := newFalAdapter(t)
	got := a.SupportedClientTypes()
	want := map[domain.ClientType]bool{domain.ClientTypeOpenAI: true, domain.ClientTypeVideo: true}
	if len(got) != 2 {
		t.Fatalf("SupportedClientTypes = %v, want openai+video", got)
	}
	for _, ct := range got {
		if !want[ct] {
			t.Fatalf("unexpected client type %q", ct)
		}
	}
}

// ---- Image ----

func TestImageGenerationURLAuthAndTranslation(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey, gotGoog, gotProxyAuth string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotGoog = r.Header.Get("x-goog-api-key")
		gotProxyAuth = r.Header.Get("Proxy-Authorization")
		gotBody, _ = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/out.jpg","width":1024,"height":1024}],"seed":42,"prompt":"a red cube"}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	body := []byte(`{"model":"ignored","prompt":"a red cube","size":"512x768","num_inference_steps":4,"response_format":"url"}`)
	c, rec := newCtx(http.MethodPost, "/v1/images/generations", body, domain.ClientTypeOpenAI)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev")

	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// URL path = mapped model (slashes preserved).
	if gotPath != "/fal-ai/flux/dev" {
		t.Fatalf("upstream path = %q, want /fal-ai/flux/dev", gotPath)
	}
	// fal auth header set, client auth stripped.
	if gotAuth != "Key "+testKey {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Key "+testKey)
	}
	if gotAPIKey != "" || gotGoog != "" || gotProxyAuth != "" {
		t.Fatalf("client auth leaked: x-api-key=%q x-goog=%q proxy=%q", gotAPIKey, gotGoog, gotProxyAuth)
	}
	// size -> image_size{width,height}; OpenAI-only fields stripped; extras kept.
	if w := gjson.GetBytes(gotBody, "image_size.width").Int(); w != 512 {
		t.Fatalf("image_size.width = %d, want 512", w)
	}
	if h := gjson.GetBytes(gotBody, "image_size.height").Int(); h != 768 {
		t.Fatalf("image_size.height = %d, want 768", h)
	}
	if gjson.GetBytes(gotBody, "model").Exists() || gjson.GetBytes(gotBody, "size").Exists() ||
		gjson.GetBytes(gotBody, "response_format").Exists() {
		t.Fatalf("OpenAI-only fields not stripped: %s", gotBody)
	}
	if n := gjson.GetBytes(gotBody, "num_inference_steps").Int(); n != 4 {
		t.Fatalf("num_inference_steps not passed through: %s", gotBody)
	}
	// Response: fal images -> OpenAI data[].url
	out := rec.Body.Bytes()
	if u := gjson.GetBytes(out, "data.0.url").String(); u != "https://fal.media/out.jpg" {
		t.Fatalf("data.0.url = %q, want fal media url; body=%s", u, out)
	}
	if !gjson.GetBytes(out, "created").Exists() {
		t.Fatalf("response missing created: %s", out)
	}
}

func TestImageNativeImageSizeWins(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = readAll(r)
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/x.jpg"}]}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	// Client sent a fal-native image_size preset AND an OpenAI size — the preset wins.
	body := []byte(`{"prompt":"p","image_size":"square_hd","size":"512x512"}`)
	c, _ := newCtx(http.MethodPost, "/v1/images/generations", body, domain.ClientTypeOpenAI)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/schnell")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if s := gjson.GetBytes(gotBody, "image_size").String(); s != "square_hd" {
		t.Fatalf("image_size = %q, want square_hd (native preset preserved); body=%s", s, gotBody)
	}
}

func TestImageB64JsonFetchesBytes(t *testing.T) {
	imgBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x01, 0x02, 0x03}
	mux := http.NewServeMux()
	mux.HandleFunc("/media/img.png", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imgBytes)
	})
	var falBaseURL string
	mux.HandleFunc("/fal-ai/flux/dev", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"images":[{"url":"` + falBaseURL + `/media/img.png"}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	falBaseURL = server.URL
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	body := []byte(`{"prompt":"p","response_format":"b64_json"}`)
	c, rec := newCtx(http.MethodPost, "/v1/images/generations", body, domain.ClientTypeOpenAI)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := rec.Body.Bytes()
	if gjson.GetBytes(out, "data.0.url").Exists() {
		t.Fatalf("b64_json response should not carry url: %s", out)
	}
	got := gjson.GetBytes(out, "data.0.b64_json").String()
	want := base64.StdEncoding.EncodeToString(imgBytes)
	if got != want {
		t.Fatalf("b64_json = %q, want %q", got, want)
	}
}

// ---- Image edit (image-to-image) ----

// newMultipartCtx builds a flow.Ctx whose request is a multipart/form-data
// images/edits upload. It sets the real multipart Content-Type on both the
// underlying *http.Request (so the adapter can parse the boundary) and the stashed
// KeyRequestHeaders, mirroring the proxy.
func newMultipartCtx(uri string, fields map[string]string, files map[string]multipartFile) (*flow.Ctx, *httptest.ResponseRecorder) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, f := range files {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition",
			`form-data; name="`+name+`"; filename="`+f.filename+`"`)
		if f.contentType != "" {
			hdr.Set("Content-Type", f.contentType)
		}
		part, _ := mw.CreatePart(hdr)
		_, _ = part.Write(f.data)
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "http://localhost"+uri, bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer sk-maxx-client-token")
	req.Header.Set("x-api-key", "sk-maxx-client-token")
	rec := httptest.NewRecorder()
	c := flow.NewCtx(rec, req)
	c.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	c.Set(flow.KeyRequestURI, uri)
	c.Set(flow.KeyRequestBody, buf.Bytes())
	c.Set(flow.KeyRequestHeaders, req.Header)
	return c, rec
}

type multipartFile struct {
	filename    string
	contentType string
	data        []byte
}

func TestImageEditMultipartToDataURI(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotBody, _ = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/edited.jpg"}],"prompt":"make it red"}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	imgBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0xff, 0x00, 0x11}
	a := newFalAdapter(t)
	c, rec := newMultipartCtx("/v1/images/edits",
		map[string]string{
			"model":           "ignored",
			"prompt":          "make it red",
			"size":            "512x768",
			"strength":        "0.85",
			"response_format": "url",
		},
		map[string]multipartFile{
			"image": {filename: "in.png", contentType: "image/png", data: imgBytes},
		},
	)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev/image-to-image")

	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotPath != "/fal-ai/flux/dev/image-to-image" {
		t.Fatalf("upstream path = %q", gotPath)
	}
	if gotAuth != "Key "+testKey {
		t.Fatalf("Authorization = %q, want fal key", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("client x-api-key leaked upstream: %q", gotAPIKey)
	}
	// image → image_url data: URI carrying the exact base64 of the upload.
	wantURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgBytes)
	if u := gjson.GetBytes(gotBody, "image_url").String(); u != wantURI {
		t.Fatalf("image_url = %q, want data URI %q", u, wantURI)
	}
	// prompt / strength / size translated; OpenAI-only fields absent.
	if p := gjson.GetBytes(gotBody, "prompt").String(); p != "make it red" {
		t.Fatalf("prompt = %q", p)
	}
	if s := gjson.GetBytes(gotBody, "strength").Float(); s != 0.85 {
		t.Fatalf("strength = %v, want 0.85 (numeric); body=%s", s, gotBody)
	}
	if w := gjson.GetBytes(gotBody, "image_size.width").Int(); w != 512 {
		t.Fatalf("image_size.width = %d, want 512", w)
	}
	if h := gjson.GetBytes(gotBody, "image_size.height").Int(); h != 768 {
		t.Fatalf("image_size.height = %d, want 768", h)
	}
	if gjson.GetBytes(gotBody, "response_format").Exists() ||
		gjson.GetBytes(gotBody, "size").Exists() ||
		gjson.GetBytes(gotBody, "model").Exists() {
		t.Fatalf("OpenAI-only fields not stripped: %s", gotBody)
	}
	// Response translated to OpenAI images shape.
	out := rec.Body.Bytes()
	if u := gjson.GetBytes(out, "data.0.url").String(); u != "https://fal.media/edited.jpg" {
		t.Fatalf("data.0.url = %q; body=%s", u, out)
	}
}

func TestImageEditMultipartMaskToMaskURL(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = readAll(r)
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/x.jpg"}]}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	maskBytes := []byte{0x01, 0x02, 0x03, 0x04}
	c, _ := newMultipartCtx("/v1/images/edits",
		map[string]string{"prompt": "inpaint"},
		map[string]multipartFile{
			"image": {filename: "in.png", contentType: "image/png", data: []byte{0x89, 0x50}},
			"mask":  {filename: "m.png", contentType: "image/png", data: maskBytes},
		},
	)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev/image-to-image")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantMask := "data:image/png;base64," + base64.StdEncoding.EncodeToString(maskBytes)
	if m := gjson.GetBytes(gotBody, "mask_url").String(); m != wantMask {
		t.Fatalf("mask_url = %q, want %q", m, wantMask)
	}
}

func TestImageEditJSONImageURLPassthrough(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = readAll(r)
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/y.jpg"}]}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	body := []byte(`{"model":"ignored","prompt":"make it blue","image_url":"https://example.com/cat.jpg","strength":0.9,"size":"256x256"}`)
	c, rec := newCtx(http.MethodPost, "/v1/images/edits", body, domain.ClientTypeOpenAI)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev/image-to-image")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if u := gjson.GetBytes(gotBody, "image_url").String(); u != "https://example.com/cat.jpg" {
		t.Fatalf("image_url passthrough = %q; body=%s", u, gotBody)
	}
	if p := gjson.GetBytes(gotBody, "prompt").String(); p != "make it blue" {
		t.Fatalf("prompt = %q", p)
	}
	if w := gjson.GetBytes(gotBody, "image_size.width").Int(); w != 256 {
		t.Fatalf("size not translated: %s", gotBody)
	}
	if gjson.GetBytes(gotBody, "model").Exists() {
		t.Fatalf("model not stripped: %s", gotBody)
	}
	out := rec.Body.Bytes()
	if u := gjson.GetBytes(out, "data.0.url").String(); u != "https://fal.media/y.jpg" {
		t.Fatalf("data.0.url = %q", u)
	}
}

func TestImageEditJSONBase64ToDataURI(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = readAll(r)
		_, _ = w.Write([]byte(`{"images":[{"url":"https://fal.media/z.jpg"}]}`))
	}))
	defer server.Close()
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	raw := []byte{0xde, 0xad, 0xbe, 0xef}
	b64 := base64.StdEncoding.EncodeToString(raw)
	a := newFalAdapter(t)
	body := []byte(`{"prompt":"edit","image_b64":"` + b64 + `"}`)
	c, _ := newCtx(http.MethodPost, "/v1/images/edits", body, domain.ClientTypeOpenAI)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev/image-to-image")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantURI := "data:image/png;base64," + b64
	if u := gjson.GetBytes(gotBody, "image_url").String(); u != wantURI {
		t.Fatalf("image_url = %q, want %q", u, wantURI)
	}
	if gjson.GetBytes(gotBody, "image_b64").Exists() {
		t.Fatalf("image_b64 not stripped: %s", gotBody)
	}
}

func TestImageEditB64JSONResponseFormat(t *testing.T) {
	imgBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0a, 0x0b}
	mux := http.NewServeMux()
	var falBaseURL string
	mux.HandleFunc("/media/edited.png", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imgBytes)
	})
	mux.HandleFunc("/fal-ai/flux/dev/image-to-image", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"images":[{"url":"` + falBaseURL + `/media/edited.png"}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	falBaseURL = server.URL
	t.Setenv("MAXX_FAL_BASE_URL", server.URL)

	a := newFalAdapter(t)
	c, rec := newMultipartCtx("/v1/images/edits",
		map[string]string{"prompt": "p", "response_format": "b64_json"},
		map[string]multipartFile{
			"image": {filename: "in.png", contentType: "image/png", data: []byte{0x01}},
		},
	)
	c.Set(flow.KeyMappedModel, "fal-ai/flux/dev/image-to-image")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := rec.Body.Bytes()
	if gjson.GetBytes(out, "data.0.url").Exists() {
		t.Fatalf("b64_json response should not carry url: %s", out)
	}
	got := gjson.GetBytes(out, "data.0.b64_json").String()
	want := base64.StdEncoding.EncodeToString(imgBytes)
	if got != want {
		t.Fatalf("b64_json = %q, want %q", got, want)
	}
}

func TestIsImageEditPath(t *testing.T) {
	cases := map[string]bool{
		"/v1/images/edits":       true,
		"/images/edits":          true,
		"/v1/images/edits/":      true,
		"/v1/images/edits?x=1":   true,
		"/v1/images/generations": false,
		"/v1/images/edits/extra": false,
	}
	for in, want := range cases {
		if got := isImageEditPath(in); got != want {
			t.Fatalf("isImageEditPath(%q) = %v, want %v", in, got, want)
		}
	}
}

// ---- Video submit ----

func TestVideoSubmitEncodesTaskAndStripsAuth(t *testing.T) {
	var gotPath, gotAuth, gotAPIKey string
	var gotBody []byte
	var queueBase string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		gotBody, _ = readAll(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"IN_QUEUE","request_id":"req-123","response_url":"` +
			queueBase + `/fal-ai/veo3/requests/req-123"}`))
	}))
	defer server.Close()
	queueBase = server.URL
	t.Setenv("MAXX_FAL_QUEUE_BASE_URL", server.URL)

	a := newFalAdapter(t)
	body := []byte(`{"model":"ignored","prompt":"a cat walking","duration":"8s"}`)
	c, rec := newCtx(http.MethodPost, "/v1/video/generations", body, domain.ClientTypeVideo)
	c.Set(flow.KeyMappedModel, "fal-ai/veo3/fast")
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotPath != "/fal-ai/veo3/fast" {
		t.Fatalf("queue submit path = %q, want /fal-ai/veo3/fast", gotPath)
	}
	if gotAuth != "Key "+testKey {
		t.Fatalf("Authorization = %q, want fal key", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("client x-api-key leaked upstream: %q", gotAPIKey)
	}
	if gjson.GetBytes(gotBody, "model").Exists() {
		t.Fatalf("model not stripped from fal input: %s", gotBody)
	}
	if p := gjson.GetBytes(gotBody, "prompt").String(); p != "a cat walking" {
		t.Fatalf("prompt not passed through: %s", gotBody)
	}

	out := rec.Body.Bytes()
	taskID := gjson.GetBytes(out, "task_id").String()
	if taskID == "" || gjson.GetBytes(out, "id").String() != taskID {
		t.Fatalf("submit response task id malformed: %s", out)
	}
	// task_id round-trips to the fal response_url.
	parts, err := decodeTaskID(taskID)
	if err != nil {
		t.Fatalf("decodeTaskID: %v", err)
	}
	wantURL := queueBase + "/fal-ai/veo3/requests/req-123"
	if len(parts) == 0 || parts[0] != wantURL {
		t.Fatalf("decoded task id = %v, want response_url %q", parts, wantURL)
	}
}

// ---- Video poll ----

func TestVideoPollCompletes(t *testing.T) {
	var statusHits, resultHits int
	var gotStatusAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/fal-ai/veo3/requests/req-123/status", func(w http.ResponseWriter, r *http.Request) {
		statusHits++
		gotStatusAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":"COMPLETED","request_id":"req-123"}`))
	})
	mux.HandleFunc("/fal-ai/veo3/requests/req-123", func(w http.ResponseWriter, r *http.Request) {
		resultHits++
		_, _ = w.Write([]byte(`{"video":{"url":"https://fal.media/final.mp4","content_type":"video/mp4"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	responseURL := server.URL + "/fal-ai/veo3/requests/req-123"
	taskID := encodeTaskID(responseURL)

	a := newFalAdapter(t)
	uri := "/v1/video/generations/" + taskID
	c, rec := newCtx(http.MethodGet, uri, nil, domain.ClientTypeVideo)
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if statusHits != 1 || resultHits != 1 {
		t.Fatalf("expected 1 status + 1 result hit, got %d/%d", statusHits, resultHits)
	}
	if gotStatusAuth != "Key "+testKey {
		t.Fatalf("status Authorization = %q, want fal key", gotStatusAuth)
	}
	out := rec.Body.Bytes()
	if s := gjson.GetBytes(out, "data.status").String(); s != statusSuccess {
		t.Fatalf("data.status = %q, want %q; body=%s", s, statusSuccess, out)
	}
	if u := gjson.GetBytes(out, "data.data.url").String(); u != "https://fal.media/final.mp4" {
		t.Fatalf("lifted mp4 url = %q; body=%s", u, out)
	}
	if u := gjson.GetBytes(out, "data.data.video.url").String(); u != "https://fal.media/final.mp4" {
		t.Fatalf("nested fal payload not preserved; body=%s", out)
	}
}

func TestVideoPollInProgress(t *testing.T) {
	mux := http.NewServeMux()
	var resultHit bool
	mux.HandleFunc("/fal-ai/veo3/requests/req-9/status", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"IN_PROGRESS","request_id":"req-9"}`))
	})
	mux.HandleFunc("/fal-ai/veo3/requests/req-9", func(w http.ResponseWriter, r *http.Request) {
		resultHit = true
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	taskID := encodeTaskID(server.URL + "/fal-ai/veo3/requests/req-9")
	a := newFalAdapter(t)
	c, rec := newCtx(http.MethodGet, "/v1/video/generations/"+taskID, nil, domain.ClientTypeVideo)
	if err := a.Execute(c, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resultHit {
		t.Fatalf("must NOT fetch result while IN_PROGRESS")
	}
	if s := gjson.GetBytes(rec.Body.Bytes(), "data.status").String(); s != statusInProgress {
		t.Fatalf("data.status = %q, want %q", s, statusInProgress)
	}
}

func TestVideoPollInvalidTaskID(t *testing.T) {
	a := newFalAdapter(t)
	c, _ := newCtx(http.MethodGet, "/v1/video/generations/!!!not-base64!!!", nil, domain.ClientTypeVideo)
	err := a.Execute(c, &domain.Provider{})
	if err == nil {
		t.Fatalf("expected error for invalid task id")
	}
	if pe, ok := err.(*domain.ProxyError); ok && pe.Scope != domain.ScopeRequest {
		t.Fatalf("invalid task id should be request-scoped, got %v", pe.Scope)
	}
}

func TestParseWxH(t *testing.T) {
	cases := []struct {
		in   string
		w, h int
		ok   bool
	}{
		{"1024x1024", 1024, 1024, true},
		{"512x768", 512, 768, true},
		{"square", 0, 0, false},
		{"axb", 0, 0, false},
		{"0x0", 0, 0, false},
	}
	for _, tc := range cases {
		w, h, ok := parseWxH(tc.in)
		if ok != tc.ok || w != tc.w || h != tc.h {
			t.Fatalf("parseWxH(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.in, w, h, ok, tc.w, tc.h, tc.ok)
		}
	}
}

func TestExtractVideoURLFallback(t *testing.T) {
	// Non-standard fal shape: url nested under a differently-named object.
	body := []byte(`{"output":{"url":"https://fal.media/z.mp4"}}`)
	if got := extractVideoURL(body); got != "https://fal.media/z.mp4" {
		t.Fatalf("extractVideoURL fallback = %q", got)
	}
}

// readAll reads a request body for assertions.
func readAll(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return io.ReadAll(r.Body)
}

// Guard: the encoded task id is URL-safe (no '/', '+', '=' padding) so it can be
// carried verbatim in the poll path.
func TestTaskIDIsURLSafe(t *testing.T) {
	id := encodeTaskID("https://queue.fal.run/fal-ai/veo3/requests/abc-123")
	if strings.ContainsAny(id, "/+=") {
		t.Fatalf("task id not URL-safe: %q", id)
	}
	if _, err := decodeTaskID(id); err != nil {
		t.Fatalf("decode round-trip failed: %v", err)
	}
}

// ---- Error classification: 403 balance/lock vs genuine auth failure ----

// TestDoJSONBalanceLockedNotAuthFailure guards the real incident: fal returns
// HTTP 403 with a "User is locked. Reason: TOP_UP." / "Exhausted balance." detail
// when an account runs out of credit. That must classify as the lighter
// insufficient_balance reason (short, self-recovering cooldown), NOT the 1h
// auth_failure cooldown, while genuine credential problems still classify as
// auth_failure.
func TestDoJSONBalanceLockedNotAuthFailure(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantScope  domain.ErrorScope
		wantReason domain.CooldownReason
	}{
		{
			name:       "403 TOP_UP lock",
			status:     http.StatusForbidden,
			body:       `{"detail":"User is locked. Reason: TOP_UP."}`,
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonInsufficientBalance,
		},
		{
			name:       "403 exhausted balance",
			status:     http.StatusForbidden,
			body:       `{"detail":"User is locked. Reason: Exhausted balance. Top up your balance at fal.ai/dashboard/billing."}`,
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonInsufficientBalance,
		},
		{
			name:       "402 insufficient balance",
			status:     http.StatusPaymentRequired,
			body:       `{"detail":"insufficient balance"}`,
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonInsufficientBalance,
		},
		{
			name:       "403 genuine auth failure (no billing signal)",
			status:     http.StatusForbidden,
			body:       `{"detail":"Forbidden: invalid key"}`,
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonAuthFailure,
		},
		{
			name:       "401 always auth failure",
			status:     http.StatusUnauthorized,
			body:       `{"detail":"Unauthorized. Reason: TOP_UP."}`, // even with a billing-ish word, 401 stays auth
			wantScope:  domain.ScopeKey,
			wantReason: domain.CooldownReasonAuthFailure,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			a := newFalAdapter(t)
			_, status, err := a.doJSON(t.Context(), http.MethodPost, server.URL+"/fal-ai/flux/dev", []byte(`{}`))
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			pe, ok := err.(*domain.ProxyError)
			if !ok {
				t.Fatalf("error = %T (%v), want *domain.ProxyError", err, err)
			}
			if pe.Scope != tc.wantScope {
				t.Fatalf("scope = %q, want %q", pe.Scope, tc.wantScope)
			}
			if pe.Reason != tc.wantReason {
				t.Fatalf("reason = %q, want %q", pe.Reason, tc.wantReason)
			}
			if pe.Retryable {
				t.Fatalf("balance/auth errors must be non-retryable, got Retryable=true")
			}
		})
	}
}

func TestIsFalBalanceLocked(t *testing.T) {
	cases := []struct {
		code int
		body string
		want bool
	}{
		{http.StatusForbidden, `{"detail":"User is locked. Reason: TOP_UP."}`, true},
		{http.StatusForbidden, `{"detail":"Exhausted balance."}`, true},
		{http.StatusForbidden, `{"detail":"TOP UP your balance"}`, true},
		{http.StatusPaymentRequired, `{"detail":"insufficient funds"}`, true},
		{http.StatusForbidden, `{"detail":"invalid api key"}`, false},
		{http.StatusUnauthorized, `{"detail":"User is locked. Reason: TOP_UP."}`, false}, // 401 never billing
		{http.StatusTooManyRequests, `{"detail":"balance"}`, false},                      // wrong status
	}
	for _, tc := range cases {
		if got := isFalBalanceLocked(tc.code, []byte(tc.body)); got != tc.want {
			t.Fatalf("isFalBalanceLocked(%d, %q) = %v, want %v", tc.code, tc.body, got, tc.want)
		}
	}
}
