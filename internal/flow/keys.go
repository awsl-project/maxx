package flow

const (
	KeyProxyContext      = "proxy_context"
	KeyProxyStream       = "proxy_stream"
	KeyProxyRequestModel = "proxy_request_model"
	KeyExecutorState     = "executor_state"

	KeyClientType          = "client_type"
	KeyOriginalClientType  = "original_client_type"
	KeySessionID           = "session_id"
	KeyProjectID           = "project_id"
	KeyRequestModel        = "request_model"
	KeyMappedModel         = "mapped_model"
	KeyRequestBody         = "request_body"
	KeyOriginalRequestBody = "original_request_body"
	KeyRequestHeaders      = "request_headers"
	KeyRequestURI          = "request_uri"
	// KeyResponsesClientPath holds the client's original Responses API request URI
	// — path + query (e.g. "/v1/responses" or "/responses?source=codex-cli") —
	// captured before any /v1 normalization, so a custom Codex downstream can be
	// forwarded the path the client actually used instead of a hardcoded one.
	KeyResponsesClientPath = "responses_client_path"
	KeyIsStream            = "is_stream"
	KeyAPITokenID          = "api_token_id"
	KeyAPITokenDevMode     = "api_token_dev_mode"
	KeyProxyRequest        = "proxy_request"
	KeyUpstreamAttempt     = "upstream_attempt"
	KeyEventChan           = "event_chan"
	KeyBroadcaster         = "broadcaster"
)
