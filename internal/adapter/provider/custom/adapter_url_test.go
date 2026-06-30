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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildUpstreamURL(tt.baseURL, tt.requestPath); got != tt.want {
				t.Fatalf("buildUpstreamURL(%q, %q) = %q, want %q", tt.baseURL, tt.requestPath, got, tt.want)
			}
		})
	}
}
