package bedrock

const (
	BedrockAPIVersion  = "bedrock-2023-05-31"
	DefaultRegion      = "us-east-1"
	DefaultModelPrefix = "us"

	// URL templates
	BedrockURLTemplate = "https://bedrock-runtime.%s.amazonaws.com"
	InvokePath         = "/model/%s/invoke"
	InvokeStreamPath   = "/model/%s/invoke-with-response-stream"
)
