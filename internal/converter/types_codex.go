package converter

// Codex API types (OpenAI Responses API)

type CodexRequest struct {
	Model              string                 `json:"model"`
	Input              interface{}            `json:"input"` // string or []InputItem
	Instructions       string                 `json:"instructions,omitempty"`
	MaxOutputTokens    int                    `json:"max_output_tokens,omitempty"`
	Temperature        *float64               `json:"temperature,omitempty"`
	TopP               *float64               `json:"top_p,omitempty"`
	Stream             bool                   `json:"stream,omitempty"`
	Tools              []CodexTool            `json:"tools,omitempty"`
	ToolChoice         interface{}            `json:"tool_choice,omitempty"`
	Reasoning          *CodexReasoning        `json:"reasoning,omitempty"`
	ServiceTier        string                 `json:"service_tier,omitempty"`
	ParallelToolCalls  *bool                  `json:"parallel_tool_calls,omitempty"`
	Include            []string               `json:"include,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	Store              bool                   `json:"store,omitempty"`
	PreviousResponseID string                 `json:"previous_response_id,omitempty"`
}

type CodexInputItem struct {
	Type      string      `json:"type"`
	Role      string      `json:"role,omitempty"`
	Content   interface{} `json:"content,omitempty"` // string or []ContentPart
	ID        string      `json:"id,omitempty"`
	CallID    string      `json:"call_id,omitempty"`
	Output    string      `json:"output,omitempty"`
	Name      string      `json:"name,omitempty"`      // for function_call
	Arguments string      `json:"arguments,omitempty"` // for function_call
}

type CodexTool struct {
	Type        string      `json:"type"`
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type CodexReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type CodexResponse struct {
	ID        string        `json:"id"`
	Object    string        `json:"object"`
	CreatedAt int64         `json:"created_at"`
	Model     string        `json:"model"`
	Output    []CodexOutput `json:"output"`
	Status    string        `json:"status"`
	Usage     CodexUsage    `json:"usage"`
	Error     *CodexError   `json:"error,omitempty"`
}

type CodexOutput struct {
	Type      string      `json:"type"`
	ID        string      `json:"id,omitempty"`
	Role      string      `json:"role,omitempty"`
	Content   interface{} `json:"content,omitempty"` // string or []ContentPart
	Name      string      `json:"name,omitempty"`
	CallID    string      `json:"call_id,omitempty"`
	Arguments string      `json:"arguments,omitempty"`
	Status    string      `json:"status,omitempty"`
}

type CodexUsage struct {
	InputTokens         int                `json:"input_tokens"`
	OutputTokens        int                `json:"output_tokens"`
	TotalTokens         int                `json:"total_tokens"`
	InputTokensDetails  *CodexTokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *CodexTokenDetails `json:"output_tokens_details,omitempty"`
}

type CodexTokenDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	TextTokens   int `json:"text_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

type CodexError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Codex streaming events
type CodexStreamEvent struct {
	Type     string         `json:"type"`
	Response *CodexResponse `json:"response,omitempty"`
	Item     *CodexOutput   `json:"item,omitempty"`
	Delta    *CodexDelta    `json:"delta,omitempty"`
}

type CodexDelta struct {
	Type      string `json:"type,omitempty"`
	Text      string `json:"text,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
