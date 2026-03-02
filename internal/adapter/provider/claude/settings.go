package claude

// Anthropic OAuth 配置
const (
	// OAuth URLs
	ClaudeAuthURL  = "https://claude.ai/oauth/authorize"
	ClaudeTokenURL = "https://api.anthropic.com/v1/oauth/token"

	// OAuth Client ID (from Claude Code CLI)
	OAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	// OAuth Scopes
	OAuthScopes = "org:create_api_key user:profile user:inference"

	// Fixed OAuth Callback
	OAuthCallbackPort = 1456
	OAuthRedirectURI  = "http://localhost:1456/auth/callback"

	// Claude API Base URL
	ClaudeBaseURL = "https://api.anthropic.com"

	// API Version
	ClaudeAPIVersion = "2023-06-01"

	// User-Agent (mimics Claude Code CLI)
	ClaudeUserAgent = "claude-cli/2.1.63 (external, cli)"

	// Anthropic Beta features
	ClaudeBetaHeader = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-scope-2026-01-05"
)
