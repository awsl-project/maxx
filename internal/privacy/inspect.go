package privacy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

func decode(body string) (any, error) {
	d := json.NewDecoder(strings.NewReader(body))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("expected one JSON value")
	}
	return value, nil
}

// InspectRequest checks message content and tool results, not authentication
// headers, model names, tool schemas or opaque protocol identifiers. Inspection
// does not modify the request. Unparseable/unsupported input fails closed when
// project protection is enabled. Binary file attachments are not inspected.
func InspectRequest(body []byte, contentType string) (string, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return "", errors.New("invalid content type")
	}
	if mediaType == "multipart/form-data" {
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return "", nil
			}
			if err != nil {
				return "", errors.New("invalid multipart body")
			}
			if part.FileName() != "" {
				continue
			}
			text, err := io.ReadAll(part)
			if err != nil {
				return "", errors.New("invalid multipart field")
			}
			if kind := inspect(string(text), false); kind != "" {
				return kind, nil
			}
		}
	}
	if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return "", errors.New("unsupported request body")
	}
	value, err := decode(string(body))
	if err != nil {
		return "", errors.New("invalid JSON body")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return "", errors.New("expected JSON object")
	}
	for _, key := range []string{"messages", "system", "input", "instructions", "contents", "systemInstruction", "system_instruction", "prompt"} {
		if kind := inspect(object[key], true); kind != "" {
			return kind, nil
		}
	}
	return "", nil
}

func inspect(value any, protocol bool) string {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if sensitiveField(k, child) && child != nil && child != "" && child != Redacted {
				return "credential_field"
			}
			if protocol {
				switch k {
				case "input", "args", "arguments", "response", "output":
					// Tool data uses arbitrary keys, including protocol-like names.
					if kind := inspect(child, false); kind != "" {
						return kind
					}
					continue
				case "id", "call_id", "tool_call_id", "type", "role", "name", "signature", "thoughtSignature", "encrypted_content", "image_url", "image", "audio", "inlineData", "inline_data", "file_data", "file_id", "file_url", "file_data_url", "data":
					continue
				}
			}
			if kind := inspect(child, protocol); kind != "" {
				return kind
			}
		}
	case []any:
		for _, child := range v {
			if kind := inspect(child, protocol); kind != "" {
				return kind
			}
		}
	case string:
		if kind := Detect(v); kind != "" {
			return kind
		}
		if nested, err := decode(v); err == nil {
			return inspect(nested, false)
		}
	}
	return ""
}
