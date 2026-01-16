package kiro

// ContentType 内容类型
type ContentType string

const (
	ContentTypeMarkdown ContentType = "markdown"
	ContentTypePlain    ContentType = "plain"
	ContentTypeJSON     ContentType = "json"
)

// MessageStatus 消息状态
type MessageStatus string

const (
	MessageStatusCompleted  MessageStatus = "Completed"
	MessageStatusInProgress MessageStatus = "InProgress"
	MessageStatusError      MessageStatus = "Error"
)

// UserIntent 用户意图
type UserIntent string

const (
	UserIntentExplainCodeSelection    UserIntent = "EXPLAIN_CODE_SELECTION"
	UserIntentSuggestAlternateImpl    UserIntent = "SUGGEST_ALTERNATE_IMPLEMENTATION"
	UserIntentApplyCommonBestPractices UserIntent = "APPLY_COMMON_BEST_PRACTICES"
	UserIntentImproveCode             UserIntent = "IMPROVE_CODE"
	UserIntentShowExamples            UserIntent = "SHOW_EXAMPLES"
	UserIntentCiteSources             UserIntent = "CITE_SOURCES"
	UserIntentExplainLineByLine       UserIntent = "EXPLAIN_LINE_BY_LINE"
)

// CodeWhispererRequest 表示 CodeWhisperer API 的请求结构
type CodeWhispererRequest struct {
	ConversationState struct {
		AgentContinuationId string `json:"agentContinuationId"`
		AgentTaskType       string `json:"agentTaskType"`
		ChatTriggerType     string `json:"chatTriggerType"`
		CurrentMessage      struct {
			UserInputMessage struct {
				UserInputMessageContext struct {
					ToolResults []ToolResult        `json:"toolResults,omitempty"`
					Tools       []CodeWhispererTool `json:"tools,omitempty"`
				} `json:"userInputMessageContext"`
				Content string               `json:"content"`
				ModelId string               `json:"modelId"`
				Images  []CodeWhispererImage `json:"images"`
				Origin  string               `json:"origin"`
			} `json:"userInputMessage"`
		} `json:"currentMessage"`
		ConversationId string `json:"conversationId"`
		History        []any  `json:"history"`
	} `json:"conversationState"`
}

// CodeWhispererImage 表示 CodeWhisperer API 的图片结构
type CodeWhispererImage struct {
	Format string `json:"format"` // "jpeg", "png", "gif", "webp"
	Source struct {
		Bytes string `json:"bytes"` // base64编码的图片数据
	} `json:"source"`
}

// CodeWhispererTool 表示 CodeWhisperer API 的工具结构
type CodeWhispererTool struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

// ToolSpecification 表示工具规范的结构
type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema 表示工具输入模式的结构
type InputSchema struct {
	Json map[string]any `json:"json"`
}

// ToolResult 表示工具执行结果的结构
type ToolResult struct {
	ToolUseId string           `json:"toolUseId"`
	Content   []map[string]any `json:"content"`
	Status    string           `json:"status"` // "success" 或 "error"
	IsError   bool             `json:"isError,omitempty"`
}

// HistoryUserMessage 表示历史记录中的用户消息
type HistoryUserMessage struct {
	UserInputMessage struct {
		Content                 string               `json:"content"`
		ModelId                 string               `json:"modelId"`
		Origin                  string               `json:"origin"`
		Images                  []CodeWhispererImage `json:"images,omitempty"`
		UserInputMessageContext struct {
			ToolResults []ToolResult        `json:"toolResults,omitempty"`
			Tools       []CodeWhispererTool `json:"tools,omitempty"`
		} `json:"userInputMessageContext"`
	} `json:"userInputMessage"`
}

// HistoryAssistantMessage 表示历史记录中的助手消息
type HistoryAssistantMessage struct {
	AssistantResponseMessage struct {
		Content  string         `json:"content"`
		ToolUses []ToolUseEntry `json:"toolUses"`
	} `json:"assistantResponseMessage"`
}

// ToolUseEntry 表示工具使用条目
type ToolUseEntry struct {
	ToolUseId string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

// AssistantResponseEvent AWS CodeWhisperer 助手响应事件
type AssistantResponseEvent struct {
	ConversationID string        `json:"conversationId"`
	MessageID      string        `json:"messageId"`
	Content        string        `json:"content"`
	ContentType    ContentType   `json:"contentType,omitempty"`
	MessageStatus  MessageStatus `json:"messageStatus,omitempty"`
}

// ToolUseEvent 工具使用事件
type ToolUseEvent struct {
	ToolUseId string `json:"toolUseId"`
	Name      string `json:"name"`
	Input     any    `json:"input"` // 可以是 map[string]any 或 string
	Stop      bool   `json:"stop"`  // 工具调用是否结束
}

// SSEEvent Claude SSE 事件格式
// 匹配 kiro2api/parser 中的 SSEEvent 定义
type SSEEvent struct {
	Event string         // 事件类型 (content_block_delta, content_block_start, etc.)
	Data  map[string]any // 事件数据
}

// KiroEventTypes 事件类型常量
// 匹配 kiro2api/parser/event_types.go
var KiroEventTypes = struct {
	COMPLETION               string
	COMPLETION_CHUNK         string
	TOOL_CALL_REQUEST        string
	TOOL_CALL_ERROR          string
	SESSION_START            string
	SESSION_END              string
	ASSISTANT_RESPONSE_EVENT string
	TOOL_USE_EVENT           string
	END_OF_TURN_EVENT        string
}{
	COMPLETION:               "completion",
	COMPLETION_CHUNK:         "completionChunk",
	TOOL_CALL_REQUEST:        "toolCallRequest",
	TOOL_CALL_ERROR:          "toolCallError",
	SESSION_START:            "sessionStart",
	SESSION_END:              "sessionEnd",
	ASSISTANT_RESPONSE_EVENT: "assistantResponseEvent",
	TOOL_USE_EVENT:           "toolUseEvent",
	END_OF_TURN_EVENT:        "endOfTurnEvent",
}

// KiroMessageTypes 消息类型常量
var KiroMessageTypes = struct {
	EVENT     string
	ERROR     string
	EXCEPTION string
}{
	EVENT:     "event",
	ERROR:     "error",
	EXCEPTION: "exception",
}

// CodeEvent 代码事件
type CodeEvent struct {
	Content string `json:"content"`
}

// EndOfTurnEvent 结束事件
type EndOfTurnEvent struct {
	ConversationID string `json:"conversationId,omitempty"`
	MessageID      string `json:"messageId,omitempty"`
}

// Token 相关类型

// TokenInfo Token 信息
type TokenInfo struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
}

// RefreshResponse Token 刷新响应
type RefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	ExpiresIn    int    `json:"expiresIn"`
	RefreshToken string `json:"refreshToken,omitempty"`

	// Social 认证方式专用字段
	ProfileArn string `json:"profileArn,omitempty"`

	// IdC 认证方式专用字段
	TokenType string `json:"tokenType,omitempty"`
}

// RefreshRequest Social 认证方式的刷新请求
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// IdcRefreshRequest IdC 认证方式的刷新请求
type IdcRefreshRequest struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	GrantType    string `json:"grantType"`
	RefreshToken string `json:"refreshToken"`
}
