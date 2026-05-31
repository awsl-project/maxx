package domain

// BedrockDiscoveryEntry is one row of a Bedrock provider's discovered
// model catalog — the short name clients send mapped to the invoke-ready
// Bedrock ID, plus which AWS catalog (inference-profile or
// foundation-model) supplied it. Persisted per provider so the
// profileDiscoverer can warm its cache at startup instead of paying
// the AWS round-trip on the first request.
type BedrockDiscoveryEntry struct {
	ShortName string
	BedrockID string
	Source    string // "inference-profile" or "foundation-model"
}
