package flow

import (
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
)

func GetClientType(c *Ctx) domain.ClientType {
	if v, ok := c.Get(KeyClientType); ok {
		if ct, ok := v.(domain.ClientType); ok {
			return ct
		}
	}
	return ""
}

func GetOriginalClientType(c *Ctx) domain.ClientType {
	if v, ok := c.Get(KeyOriginalClientType); ok {
		if ct, ok := v.(domain.ClientType); ok {
			return ct
		}
	}
	return ""
}

func GetSessionID(c *Ctx) string {
	if v, ok := c.Get(KeySessionID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetProjectID(c *Ctx) uint64 {
	if v, ok := c.Get(KeyProjectID); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

func GetRequestModel(c *Ctx) string {
	if v, ok := c.Get(KeyRequestModel); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetMappedModel(c *Ctx) string {
	if v, ok := c.Get(KeyMappedModel); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetRequestBody(c *Ctx) []byte {
	if v, ok := c.Get(KeyRequestBody); ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	return nil
}

func GetOriginalRequestBody(c *Ctx) []byte {
	if v, ok := c.Get(KeyOriginalRequestBody); ok {
		if b, ok := v.([]byte); ok {
			return b
		}
	}
	return nil
}

func GetRequestHeaders(c *Ctx) http.Header {
	if v, ok := c.Get(KeyRequestHeaders); ok {
		if h, ok := v.(http.Header); ok {
			return h
		}
	}
	return nil
}

func GetRequestURI(c *Ctx) string {
	if v, ok := c.Get(KeyRequestURI); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetResponsesClientPath returns the client's original Responses API path
// captured before /v1 normalization (e.g. "/v1/responses" or "/responses"),
// or "" when the request was not a Responses call.
func GetResponsesClientPath(c *Ctx) string {
	if v, ok := c.Get(KeyResponsesClientPath); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetIsStream(c *Ctx) bool {
	if v, ok := c.Get(KeyIsStream); ok {
		if s, ok := v.(bool); ok {
			return s
		}
	}
	return false
}

func GetAPITokenID(c *Ctx) uint64 {
	if v, ok := c.Get(KeyAPITokenID); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

func GetProxyRequest(c *Ctx) *domain.ProxyRequest {
	if v, ok := c.Get(KeyProxyRequest); ok {
		if pr, ok := v.(*domain.ProxyRequest); ok {
			return pr
		}
	}
	return nil
}

func GetUpstreamAttempt(c *Ctx) *domain.ProxyUpstreamAttempt {
	if v, ok := c.Get(KeyUpstreamAttempt); ok {
		if at, ok := v.(*domain.ProxyUpstreamAttempt); ok {
			return at
		}
	}
	return nil
}

func GetEventChan(c *Ctx) domain.AdapterEventChan {
	if v, ok := c.Get(KeyEventChan); ok {
		if ch, ok := v.(domain.AdapterEventChan); ok {
			return ch
		}
	}
	return nil
}

func GetBroadcaster(c *Ctx) event.Broadcaster {
	if v, ok := c.Get(KeyBroadcaster); ok {
		if b, ok := v.(event.Broadcaster); ok {
			return b
		}
	}
	return nil
}
