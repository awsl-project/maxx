package custom

import "testing"

func TestBuildUpstreamURLNormalizesOpenAIBaseRoots(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		requestPath string
		want        string
	}{
		{
			name:        "origin plus canonical v1 path",
			baseURL:     "https://api.openai.com",
			requestPath: "/v1/chat/completions",
			want:        "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "versioned root plus canonical v1 path",
			baseURL:     "https://api.openai.com/v1",
			requestPath: "/v1/chat/completions",
			want:        "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "versioned root plus root-style chat path",
			baseURL:     "https://api.openai.com/v1",
			requestPath: "/chat/completions",
			want:        "https://api.openai.com/v1/chat/completions",
		},
		{
			name:        "compat prefix plus canonical v1 path",
			baseURL:     "https://relay.example.com/compatible",
			requestPath: "/v1/chat/completions",
			want:        "https://relay.example.com/compatible/v1/chat/completions",
		},
		{
			name:        "compat versioned prefix plus canonical v1 path",
			baseURL:     "https://relay.example.com/compatible/v1",
			requestPath: "/v1/chat/completions",
			want:        "https://relay.example.com/compatible/v1/chat/completions",
		},
		{
			name:        "root-style images path gains v1",
			baseURL:     "https://api.openai.com",
			requestPath: "/images/generations",
			want:        "https://api.openai.com/v1/images/generations",
		},
		{
			// OpenRouter unified image endpoint: bare /images (provider-prefixed
			// path arrives without /v1) must gain the /v1 prefix.
			name:        "bare images path gains v1",
			baseURL:     "https://openrouter.ai/api",
			requestPath: "/images",
			want:        "https://openrouter.ai/api/v1/images",
		},
		{
			// Canonical /v1/images route composes against the OpenRouter /api base.
			name:        "canonical v1 images path on openrouter base",
			baseURL:     "https://openrouter.ai/api",
			requestPath: "/v1/images",
			want:        "https://openrouter.ai/api/v1/images",
		},
		{
			name:        "root-style video submit path gains v1",
			baseURL:     "https://code0.ai",
			requestPath: "/video/generations",
			want:        "https://code0.ai/v1/video/generations",
		},
		{
			name:        "root-style video poll path gains v1",
			baseURL:     "https://code0.ai",
			requestPath: "/video/generations/task_abc",
			want:        "https://code0.ai/v1/video/generations/task_abc",
		},
		{
			name:        "canonical v1 video poll path unchanged",
			baseURL:     "https://code0.ai",
			requestPath: "/v1/video/generations/task_abc",
			want:        "https://code0.ai/v1/video/generations/task_abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildUpstreamURL(tt.baseURL, tt.requestPath); got != tt.want {
				t.Fatalf("buildUpstreamURL(%q, %q) = %q, want %q", tt.baseURL, tt.requestPath, got, tt.want)
			}
		})
	}
}
