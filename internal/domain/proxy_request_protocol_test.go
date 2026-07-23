package domain

import "testing"

func TestResolveProxyRequestProtocol(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		isStream    bool
		isWebSocket bool
		want        string
	}{
		{name: "http", isStream: false, isWebSocket: false, want: ProxyRequestProtocolHTTP},
		{name: "sse", isStream: true, isWebSocket: false, want: ProxyRequestProtocolSSE},
		{name: "websocket", isStream: true, isWebSocket: true, want: ProxyRequestProtocolWebSocket},
		{name: "websocket_without_stream_flag", isStream: false, isWebSocket: true, want: ProxyRequestProtocolWebSocket},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveProxyRequestProtocol(tc.isStream, tc.isWebSocket); got != tc.want {
				t.Fatalf("ResolveProxyRequestProtocol(%v, %v) = %q, want %q", tc.isStream, tc.isWebSocket, got, tc.want)
			}
		})
	}
}
