package responsemodifier

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// new-api / one-api relays proxy Gemini image models through Google's
// OpenAI-compatibility layer, but instead of emitting the generated image in the
// standard OpenAI `message.images[]` array (the OpenRouter convention that maxx's
// converters read), they embed it as a Markdown image inside `message.content`:
//
//	"Here's a red apple!\n![image](data:image/png;base64,iVBORw0K...)"
//
// A client reading `message.images[]` therefore gets nothing, and the Gemini
// response converter (openai_to_gemini) has no image to lift into an inlineData
// part. This modifier normalizes that non-standard shape back to the standard one:
// it pulls every Markdown data-URL image out of the assistant content and into
// `message.images[]`, leaving the surrounding prose intact. Downstream this means an
// OpenAI client receives the standard images array, and — since it runs before the
// client-facing bytes are produced — a Gemini client's openai->gemini conversion can
// carry the image as inlineData just like it does for OpenRouter.
//
// It is scoped to new-api providers on the OpenAI protocol: a native Gemini
// (/v1beta) request to the same upstream already returns inlineData and never hits
// this path, and other provider types don't produce the Markdown shape.

// markdownDataImageRE matches a Markdown image whose URL is a base64 data URI,
// e.g. ![image](data:image/png;base64,AAAA). Group 1 is the data URI itself.
var markdownDataImageRE = regexp.MustCompile(`!\[[^\]]*\]\(\s*(data:image/[A-Za-z0-9.+-]+;base64,[A-Za-z0-9+/=]+)\s*\)`)

// newapiSSEDataLineRE isolates the JSON payload of an SSE `data:` line so streamed
// chunks can be rewritten in place, preserving the exact prefix/newline framing.
var newapiSSEDataLineRE = regexp.MustCompile(`(?m)^(\s*data:\s*)(.*?)(\r?\n?)$`)

type newapiImageResponseModifier struct{}

func newNewapiImageResponseModifier(provider *domain.Provider, clientType domain.ClientType) *newapiImageResponseModifier {
	if provider == nil || provider.Type != "newapi" || clientType != domain.ClientTypeOpenAI {
		return nil
	}
	return &newapiImageResponseModifier{}
}

func (m *newapiImageResponseModifier) modifyBody(body []byte) []byte {
	return rewriteMarkdownImages(body)
}

func (m *newapiImageResponseModifier) modifyStreamEvent(event []byte) []byte {
	return newapiSSEDataLineRE.ReplaceAllFunc(event, func(line []byte) []byte {
		parts := newapiSSEDataLineRE.FindSubmatch(line)
		if len(parts) != 4 || bytes.Equal(bytes.TrimSpace(parts[2]), []byte("[DONE]")) {
			return line
		}
		payload := rewriteMarkdownImages(parts[2])
		if bytes.Equal(payload, parts[2]) {
			return line
		}
		out := make([]byte, 0, len(parts[1])+len(payload)+len(parts[3]))
		out = append(out, parts[1]...)
		out = append(out, payload...)
		out = append(out, parts[3]...)
		return out
	})
}

// rewriteMarkdownImages lifts Markdown data-URL images out of every choice's
// content (non-stream `message`) or incremental content (stream `delta`) into that
// object's `images[]` array. It is a no-op — returning the input unchanged — when
// there is no Markdown data image, when the body isn't a JSON object, or when a data
// URL is split across stream chunks (best effort: only complete matches are lifted).
func rewriteMarkdownImages(body []byte) []byte {
	// Fast path: skip the JSON parse only when the body cannot possibly contain a
	// data-URL image. Guard on the minimal substring every match shares ("data:image")
	// rather than the full "](data:image/" — the regex tolerates whitespace after the
	// paren (`![image](  data:...`) and JSON may escape the slash (`data:image\/png`),
	// so a stricter guard would drop matches the extraction below would otherwise make.
	if !bytes.Contains(body, []byte("data:image")) {
		return body
	}
	// UseNumber so untouched extension fields (e.g. large integer ids/usage counters)
	// round-trip byte-for-byte instead of being coerced to float64 and re-encoded with
	// precision loss. Reject trailing non-JSON data so a non-object payload is passed
	// through untouched rather than partially parsed.
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return body
	}
	if decoder.More() {
		return body
	}
	choices, ok := object["choices"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, raw := range choices {
		choice, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"message", "delta"} {
			if obj, ok := choice[key].(map[string]any); ok && liftMarkdownImages(obj) {
				changed = true
			}
		}
	}
	if !changed {
		return body
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return body
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// liftMarkdownImages moves any Markdown data-URL images from obj["content"] into
// obj["images"] (appending to any already present), returning whether it changed
// anything. The surrounding prose is preserved with the image markup removed.
func liftMarkdownImages(obj map[string]any) bool {
	content, ok := obj["content"].(string)
	if !ok || content == "" {
		return false
	}
	matches := markdownDataImageRE.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return false
	}

	images, _ := obj["images"].([]any)
	for _, match := range matches {
		images = append(images, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": match[1]},
		})
	}
	obj["images"] = images
	obj["content"] = strings.TrimSpace(markdownDataImageRE.ReplaceAllString(content, ""))
	return true
}
