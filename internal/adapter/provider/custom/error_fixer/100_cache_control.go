package error_fixer

// Upstream rejects cache_control fields (e.g. AWS Bedrock).
// Error example:
//   400 {"error":{"message":"...cache_control.***.scope: Extra inputs are not permitted..."}}
//
// Fix: strip all cache_control from system, tools, and message content blocks.

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var _ ErrorFixer = (*cacheControlFixer)(nil)

func init() {
	Register(&cacheControlFixer{})
}

type cacheControlFixer struct{}

func (f *cacheControlFixer) Name() string    { return "cache_control" }
func (f *cacheControlFixer) Priority() int { return 100 }

func (f *cacheControlFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	return bytes.Contains(body, []byte("cache_control"))
}

func (f *cacheControlFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	return req, stripAllCacheControl(body)
}

// stripAllCacheControl removes all cache_control fields from the payload.
func stripAllCacheControl(payload []byte) []byte {
	// 1. Strip from system array
	system := gjson.GetBytes(payload, "system")
	if system.IsArray() {
		for i := int(system.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("system.%d.cache_control", i)
			if gjson.GetBytes(payload, path).Exists() {
				payload, _ = sjson.DeleteBytes(payload, path)
			}
		}
	}

	// 2. Strip from tools array
	tools := gjson.GetBytes(payload, "tools")
	if tools.IsArray() {
		for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("tools.%d.cache_control", i)
			if gjson.GetBytes(payload, path).Exists() {
				payload, _ = sjson.DeleteBytes(payload, path)
			}
		}
	}

	// 3. Strip from messages content blocks
	messages := gjson.GetBytes(payload, "messages")
	if messages.IsArray() {
		for i := int(messages.Get("#").Int()) - 1; i >= 0; i-- {
			content := gjson.GetBytes(payload, fmt.Sprintf("messages.%d.content", i))
			if content.IsArray() {
				for j := int(content.Get("#").Int()) - 1; j >= 0; j-- {
					path := fmt.Sprintf("messages.%d.content.%d.cache_control", i, j)
					if gjson.GetBytes(payload, path).Exists() {
						payload, _ = sjson.DeleteBytes(payload, path)
					}
				}
			}
		}
	}

	return payload
}
