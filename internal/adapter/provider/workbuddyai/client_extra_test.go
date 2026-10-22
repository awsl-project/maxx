package workbuddyai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountPendingRetainsToken(t *testing.T) {
	tokens := 0
	accounts := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/token":
			tokens++
			fmt.Fprint(w, `{"data":{"accessToken":"dummy"}}`)
		case "/v2/plugin/login/account":
			accounts++
			if accounts == 1 {
				fmt.Fprint(w, `{"code":12151}`)
			} else {
				fmt.Fprint(w, `{"data":{"uid":"u","type":"personal"}}`)
			}
		}
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	a, done, e := c.Poll(context.Background(), "state", Config{})
	if done || e != nil || a.AccessToken != "dummy" {
		t.Fatal(a, done, e)
	}
	_, done, e = c.Poll(context.Background(), "state", a)
	if !done || e != nil || tokens != 1 {
		t.Fatal(done, e, tokens)
	}
}
func TestModelsCancellationAndMalformedEnvelope(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "not json") }))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	if _, e := c.Models(context.Background(), Config{}); e == nil {
		t.Fatal("invalid response accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Models(ctx, Config{}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
