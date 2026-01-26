package codex

// OAuth 配置 (来自 CLIProxyAPI)
const (
	// OAuth URLs
	OpenAIAuthURL  = "https://auth.openai.com/oauth/authorize"
	OpenAITokenURL = "https://auth.openai.com/oauth/token"

	// OAuth Client ID (from CLIProxyAPI)
	OAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// OAuth Scopes
	OAuthScopes = "openid email profile offline_access"

	// Fixed OAuth Callback (required by OpenAI)
	OAuthCallbackPort = 1455
	OAuthRedirectURI  = "http://localhost:1455/auth/callback"

	// Codex API Base URL
	CodexBaseURL = "https://chatgpt.com/backend-api/codex"

	// Codex Usage/Quota API URL
	CodexUsageURL = "https://chatgpt.com/backend-api/wham/usage"

	// API Version
	CodexVersion = "0.21.0"

	// User-Agent (mimics codex CLI)
	CodexUserAgent = "codex_cli_rs/0.50.0 (Mac OS 26.0.1; arm64)"

	// Originator header
	CodexOriginator = "codex_cli_rs"

	// OpenAI Beta header
	OpenAIBetaHeader = "responses=experimental"
)
