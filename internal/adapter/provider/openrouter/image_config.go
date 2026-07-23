package openrouter

import (
	"math"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeImageConfigBody rewrites an outbound OpenRouter request so an image
// aspect ratio is expressed in the form the target endpoint actually honors.
//
// This is the provider-owned half of the "each protocol sends its own standard,
// the provider converts" design. By the time a request reaches here it is in
// OpenRouter-native OpenAI shape (an OpenAI client's own body, or a Gemini client
// converted upstream), carrying the aspect ratio as either the OpenAI pixel
// `size` or the `image_config.aspect_ratio` object. Which one the upstream model
// reads depends entirely on the endpoint (verified against live OpenRouter):
//
//   - chat/completions: Gemini image models read image_config.aspect_ratio and
//     ignore size; image output also requires modalities:["image","text"].
//     (GPT image models cannot control size on chat at all — an upstream limit.)
//   - images (generations): both Gemini and GPT image models read the pixel
//     `size`; image_config is not consulted.
//
// So we fill in whichever representation the endpoint needs from whichever the
// client sent, leaving the original field intact. Returns the (possibly
// unchanged) body; never nil.
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

	// chat/completions: honor image_config.aspect_ratio, and ensure the request
	// actually asks for image output.
	if aspect == "" && size != "" {
		if a := aspectFromSize(size); a != "" {
			if out, err := sjson.SetBytes(body, "image_config.aspect_ratio", a); err == nil {
				body = out
			}
		}
	}
	return ensureImageModalities(body)
}

func isImagesEndpoint(requestURI string) bool {
	return strings.Contains(requestURI, "/images")
}

// ensureImageModalities makes sure a chat request asks for image output, which
// OpenRouter requires via modalities:["image","text"]. When modalities already
// exist without "image", it prepends "image" and preserves the rest rather than
// clobbering them (e.g. ["audio","text"] → ["image","audio","text"]).
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
// bucket. These are the sizes GPT image models accept, and OpenRouter maps them
// to the nearest native aspect for Gemini image models. Landscape/portrait/square
// is the best fidelity achievable with the standard buckets.
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

// geminiAspectRatios is the set of aspect ratios Gemini image models
// ("Nano Banana" family: 2.5 Flash Image, 3.x Flash Image) actually honor via
// image_config.aspect_ratio. Kept ordered widescreen→square→tall for readability;
// order does not affect matching.
var geminiAspectRatios = []struct {
	label string
	ratio float64
}{
	{"21:9", 21.0 / 9.0},
	{"16:9", 16.0 / 9.0},
	{"3:2", 3.0 / 2.0},
	{"4:3", 4.0 / 3.0},
	{"5:4", 5.0 / 4.0},
	{"1:1", 1.0},
	{"4:5", 4.0 / 5.0},
	{"3:4", 3.0 / 4.0},
	{"2:3", 2.0 / 3.0},
	{"9:16", 9.0 / 16.0},
}

// aspectFromSize maps a pixel size ("1920x1080") to the nearest aspect ratio
// Gemini image models honor on the chat endpoint. Matching is by log-distance so
// it is scale-free and symmetric (a ratio and its inverse are equidistant from
// square), which correctly rounds e.g. 1920x1080→16:9 and 1024x1280→4:5 instead
// of collapsing everything to a coarse landscape/portrait/square bucket.
func aspectFromSize(size string) string {
	w, h := parsePixelSize(size)
	if w == 0 || h == 0 {
		return ""
	}
	return nearestGeminiAspect(float64(w) / float64(h))
}

// nearestGeminiAspect returns the label of the supported aspect ratio closest to
// r (a width/height ratio) in log space, or "" when r is non-positive.
func nearestGeminiAspect(r float64) string {
	if r <= 0 {
		return ""
	}
	target := math.Log(r)
	best := ""
	bestDist := math.MaxFloat64
	for _, a := range geminiAspectRatios {
		if d := math.Abs(math.Log(a.ratio) - target); d < bestDist {
			bestDist = d
			best = a.label
		}
	}
	return best
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
