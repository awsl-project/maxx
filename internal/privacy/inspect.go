package privacy

import (
	"encoding/json"
	"errors"
	"io"
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

// inspectArchive checks reconstructed stream content before storing a copy.
func inspectArchive(value any) string {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if sensitiveField(k, child) && child != nil && child != "" && child != Redacted {
				return "credential_field"
			}
			if kind := inspectArchive(child); kind != "" {
				return kind
			}
		}
	case []any:
		for _, child := range v {
			if kind := inspectArchive(child); kind != "" {
				return kind
			}
		}
	case string:
		if kind := Detect(v); kind != "" {
			return kind
		}
		if nested, err := decode(v); err == nil {
			return inspectArchive(nested)
		}
	}
	return ""
}
