package bedrock

import "testing"

func TestIsBedrockModelUnavailable(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "foundation model resolution failure",
			body: `{"message":"could not resolve the foundation model from the given identifier"}`,
			want: true,
		},
		{
			name: "not authorized to perform bedrock",
			body: `{"message":"user is not authorized to perform: bedrock:invokemodel"}`,
			want: true,
		},
		{
			name: "no access to model",
			body: `{"message":"you don't have access to the model with the specified model id"}`,
			want: true,
		},
		{
			name: "inference profile error",
			body: `{"message":"the inference profile arn is invalid"}`,
			want: true,
		},
		{
			name: "field validation mentioning model — should NOT match",
			body: `{"message":"validationexception: 1 validation error: value at 'model' failed to satisfy constraint"}`,
			want: false,
		},
		{
			name: "generic validation error — should NOT match",
			body: `{"message":"validationexception: extra inputs are not permitted"}`,
			want: false,
		},
		{
			name: "tool_result error — should NOT match",
			body: `{"message":"messages.16.content.0: unexpected tool_use_id found in tool_result blocks"}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBedrockModelUnavailable(tt.body)
			if got != tt.want {
				t.Errorf("isBedrockModelUnavailable(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}
