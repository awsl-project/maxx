package handler

import "testing"

func TestIsStaticAPIPath_AllowsExactChatPage(t *testing.T) {
	if isStaticAPIPath("/chat") {
		t.Fatal("exact /chat must be served by the SPA instead of the API 404 guard")
	}
}

func TestIsStaticAPIPath_KeepsChatAPISubpathsGuarded(t *testing.T) {
	if !isStaticAPIPath("/chat/completions/extra") {
		t.Fatal("chat API subpaths must remain guarded as API paths")
	}
}
