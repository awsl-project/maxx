package mockserver

import "encoding/json"

// DefaultSuccessResponse returns a protocol-appropriate success response body.
func DefaultSuccessResponse(protocol Protocol, model string) []byte {
	var resp any
	switch protocol {
	case ProtocolClaude:
		resp = map[string]any{
			"id": "msg_mock_001", "type": "message", "role": "assistant",
			"model": model,
			"content": []map[string]any{
				{"type": "text", "text": "Hello from mock server!"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 8},
		}
	case ProtocolOpenAI:
		resp = map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion", "model": model, "created": 1700000000,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "Hello from mock server!"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18},
		}
	case ProtocolGemini:
		resp = map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": "Hello from mock server!"}},
				},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount": 10, "candidatesTokenCount": 8, "totalTokenCount": 18,
			},
		}
	case ProtocolCodex:
		resp = map[string]any{
			"id": "rsp_mock_001", "object": "response", "model": model, "created_at": 1700000000000,
			"output": []map[string]any{{
				"type": "message", "id": "msg_mock_001", "role": "assistant",
				"content": []map[string]any{{"type": "output_text", "text": "Hello from mock server!"}},
				"status":  "completed",
			}},
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 10, "output_tokens": 8, "total_tokens": 18},
		}
	}
	b, _ := json.Marshal(resp)
	return b
}

// DefaultErrorResponse returns a protocol-appropriate error response body for the given status code.
func DefaultErrorResponse(protocol Protocol, statusCode int, message string) []byte {
	var resp any
	switch protocol {
	case ProtocolClaude:
		errType := "api_error"
		switch {
		case statusCode == 429:
			errType = "rate_limit_error"
		case statusCode == 401:
			errType = "authentication_error"
		case statusCode == 404:
			errType = "not_found_error"
		case statusCode >= 500:
			errType = "api_error"
		}
		resp = map[string]any{
			"type":  "error",
			"error": map[string]any{"type": errType, "message": message},
		}
	case ProtocolGemini:
		status := "INTERNAL"
		switch {
		case statusCode == 429:
			status = "RESOURCE_EXHAUSTED"
		case statusCode == 401 || statusCode == 403:
			status = "PERMISSION_DENIED"
		case statusCode == 404:
			status = "NOT_FOUND"
		case statusCode == 503:
			status = "UNAVAILABLE"
		}
		resp = map[string]any{
			"error": map[string]any{"code": statusCode, "message": message, "status": status},
		}
	default: // OpenAI, Codex
		errType := "server_error"
		switch {
		case statusCode == 429:
			errType = "rate_limit_exceeded"
		case statusCode == 401:
			errType = "invalid_api_key"
		case statusCode == 404:
			errType = "model_not_found"
		case statusCode == 503:
			errType = "server_error"
		}
		resp = map[string]any{
			"error": map[string]any{"type": errType, "message": message, "code": errType},
		}
	}
	b, _ := json.Marshal(resp)
	return b
}
