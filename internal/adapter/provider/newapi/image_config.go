package newapi

import (
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeImageConfigBody rewrites an outbound new-api request so an image
// aspect ratio is expressed in the form a new-api / Google-OpenAI-compat upstream
// actually honors.
//
// This is the new-api half of the "each protocol sends its own standard, the
// provider converts" design — the exact mirror of the openrouter adapter, but
// aimed at a different upstream dialect. By the time a request reaches here it is
// in OpenAI shape (an OpenAI client's own body, or a Gemini client converted
// upstream), carrying the aspect ratio as either the OpenAI pixel `size` or the
// top-level `image_config` object. new-api (which proxies Gemini through Google's
// OpenAI-compatibility layer) reads neither of those on chat: it wants the aspect
// under `extra_body.google.image_config`, plus modalities:["image","text"] to ask
// for image output. Verified live against a new-api instance:
//
//	{"modalities":["image","text"],
//	 "extra_body":{"google":{"image_config":{"aspect_ratio":"16:9"}}}}  → 1344x768
//
// So we translate whichever representation the client sent into the new-api form,
// leaving the original fields intact (new-api ignores unknown keys). On the images
// endpoint new-api reads the pixel `size` like plain OpenAI, so we only derive that.
// Returns the (possibly unchanged) body; never nil.
//
// NOTE: the small aspect/size math helpers below intentionally mirror the ones in
// the openrouter package rather than sharing a module, to keep each first-class
// provider adapter self-contained and avoid touching the merged openrouter path.
func normalizeImageConfigBody(body []byte, requestURI string) []byte {
	aspect := gjson.GetBytes(body, "image_config.aspect_ratio").String()
	imageSize := gjson.GetBytes(body, "image_config.image_size").String()
	size := gjson.GetBytes(body, "size").String()
	if aspect == "" && imageSize == "" && size == "" {
		return body // no image sizing intent
	}

	if isImagesEndpoint(requestURI) {
		// The images endpoint honors pixel size; derive it from the aspect ratio
		// when the client only expressed image_config.
		if size == "" && aspect != "" {
			if s := sizeFromAspect(aspect); s != "" {
				if out, err := sjson.SetBytes(body, "size", s); err == nil {
					body = out
				}
			}
		}
		return body
	}

	// chat/completions: new-api reads the aspect ratio from
	// extra_body.google.image_config, and needs modalities to emit an image.
	if aspect == "" && size != "" {
		aspect = aspectFromSize(size)
	}
	if aspect != "" {
		if out, err := sjson.SetBytes(body, "extra_body.google.image_config.aspect_ratio", aspect); err == nil {
			body = out
		}
	}
	if imageSize != "" {
		if out, err := sjson.SetBytes(body, "extra_body.google.image_config.image_size", imageSize); err == nil {
			body = out
		}
	}
	return ensureImageModalities(body)
}

func isImagesEndpoint(requestURI string) bool {
	return strings.Contains(requestURI, "/images")
}

// ensureImageModalities makes sure a chat request asks for image output. When
// modalities already exist without "image", it prepends "image" and preserves the
// rest rather than clobbering them (e.g. ["audio","text"] → ["image","audio","text"]).
func ensureImageModalities(body []byte) []byte {
	if mods := gjson.GetBytes(body, "modalities"); mods.IsArray() {
		values := make([]string, 0, len(mods.Array())+1)
		for _, m := range mods.Array() {
			if strings.EqualFold(m.String(), "image") {
				return body // already requests image
			}
			values = append(values, m.String())
		}
		values = append([]string{"image"}, values...)
		if out, err := sjson.SetBytes(body, "modalities", values); err == nil {
			return out
		}
		return body
	}
	if out, err := sjson.SetBytes(body, "modalities", []string{"image", "text"}); err == nil {
		return out
	}
	return body
}

// sizeFromAspect maps an aspect ratio ("16:9") to a standard OpenAI pixel size
// bucket for the images endpoint. Landscape/portrait/square is the best fidelity
// achievable with the standard buckets.
func sizeFromAspect(aspect string) string {
	r := ratioOfAspect(aspect)
	switch {
	case r == 0:
		return ""
	case r >= 1.15:
		return "1536x1024"
	case r <= 0.87:
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

// aspectFromSize maps a pixel size ("1536x1024") to a coarse aspect ratio bucket
// that Gemini image models honor.
func aspectFromSize(size string) string {
	w, h := parsePixelSize(size)
	if w == 0 || h == 0 {
		return ""
	}
	r := float64(w) / float64(h)
	switch {
	case r >= 1.15:
		return "3:2"
	case r <= 0.87:
		return "2:3"
	default:
		return "1:1"
	}
}

// ratioOfAspect parses "W:H" into a width/height ratio, or 0 when unparseable.
func ratioOfAspect(aspect string) float64 {
	parts := strings.SplitN(strings.TrimSpace(aspect), ":", 2)
	if len(parts) != 2 {
		return 0
	}
	w, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	h, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || h == 0 {
		return 0
	}
	return w / h
}

// parsePixelSize parses "WxH" (e.g. "1536x1024") into width and height.
func parsePixelSize(size string) (int, int) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(size)), "x", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}
