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
			// z.ai coding plan: base already carries the /v4 version, so the
			// canonical /v1 prefix must be dropped (z.ai 404s on /paas/v4/v1/...).
			name:        "zai coding versioned root drops doubled v1",
			baseURL:     "https://api.z.ai/api/coding/paas/v4",
			requestPath: "/v1/chat/completions",
			want:        "https://api.z.ai/api/coding/paas/v4/chat/completions",
		},
		{
			// z.ai standard API: same collapse, and a root-style path still gains
			// exactly one version segment (not two).
			name:        "zai standard versioned root plus root-style chat path",
			baseURL:     "https://api.z.ai/api/paas/v4",
			requestPath: "/chat/completions",
			want:        "https://api.z.ai/api/paas/v4/chat/completions",
		},
		{
			// Model discovery via the proxy: /v1/models collapses against the
			// versioned root too.
			name:        "zai versioned root drops doubled v1 for models",
			baseURL:     "https://api.z.ai/api/paas/v4",
			requestPath: "/v1/models",
			want:        "https://api.z.ai/api/paas/v4/models",
		},
		{
			// z.ai Anthropic root ends in "anthropic" (not a version segment), so
			// the Claude /v1/messages path is preserved untouched.
			name:        "zai anthropic root keeps v1 messages",
			baseURL:     "https://api.z.ai/api/anthropic",
			requestPath: "/v1/messages",
			want:        "https://api.z.ai/api/anthropic/v1/messages",
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
			baseURL:     "https://video.example.test",
			requestPath: "/video/generations",
			want:        "https://video.example.test/v1/video/generations",
		},
		{
			name:        "root-style video poll path gains v1",
			baseURL:     "https://video.example.test",
			requestPath: "/video/generations/task_abc",
			want:        "https://video.example.test/v1/video/generations/task_abc",
		},
		{
			name:        "canonical v1 video poll path unchanged",
			baseURL:     "https://video.example.test",
			requestPath: "/v1/video/generations/task_abc",
			want:        "https://video.example.test/v1/video/generations/task_abc",
		},
		{
			name:        "root-style videos submit path gains v1",
			baseURL:     "https://video.example.test",
			requestPath: "/videos",
			want:        "https://video.example.test/v1/videos",
		},
		{
			name:        "root-style videos query path gains v1",
			baseURL:     "https://video.example.test",
			requestPath: "/videos?limit=1",
			want:        "https://video.example.test/v1/videos?limit=1",
		},
		{
			name:        "root-style videos poll path gains v1",
			baseURL:     "https://video.example.test",
			requestPath: "/videos/task_abc",
			want:        "https://video.example.test/v1/videos/task_abc",
		},
		{
			name:        "canonical v1 videos poll path unchanged",
			baseURL:     "https://video.example.test",
			requestPath: "/v1/videos/task_abc",
			want:        "https://video.example.test/v1/videos/task_abc",
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
