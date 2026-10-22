package workbuddyai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshConcurrentReusePersistenceAndCancellation(t *testing.T) {
	var calls atomic.Int32
	gate := make(chan struct{})
	started := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		if r.Header.Get("X-Refresh-Token") != "dummy" || r.Header.Get("X-Tenant-Id") != "team" {
			t.Error("refresh headers")
		}
		<-gate
		fmt.Fprint(w, `{"code":0,"data":{"accessToken":"new","refreshToken":"rotated","expiresIn":3600,"refreshExpiresIn":7200}}`)
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	a := Config{AccessToken: "old", RefreshToken: "dummy", ExpiresAt: 1, UID: "u", EnterpriseID: "team"}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, e := c.Refresh(context.Background(), a)
			if e != nil || n.AccessToken != "new" || n.UID != "u" {
				t.Errorf("refresh: %v", e)
			}
		}()
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Refresh(ctx, a); !errors.Is(e, context.Canceled) {
		t.Error(e)
	}
	close(gate)
	wg.Wait()
	n, e := c.Refresh(context.Background(), a)
	if e != nil || n.RefreshToken != "rotated" || calls.Load() != 1 {
		t.Fatal("old token not reused", e, calls.Load())
	}
	saved := false
	_, e = c.Fresh(context.Background(), a, func(_ context.Context, n Config) error {
		saved = n.AccessToken == "new"
		return errors.New("storage unavailable")
	})
	if !saved || e == nil {
		t.Fatal("persistence failure lost")
	}
}
func TestBrowserFlowEnterpriseAndModels(t *testing.T) {
	var tokens atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/plugin/auth/state":
			if r.URL.Query().Get("platform") != "workbuddy-ai" || r.Header.Get("X-No-Authorization") != "true" {
				t.Error("state headers")
			}
			fmt.Fprint(w, `{"data":{"state":"remote","authUrl":"https://example.com/login"}}`)
		case "/v2/plugin/auth/token":
			if tokens.Add(1) == 1 {
				fmt.Fprint(w, `{"code":11217}`)
			} else {
				fmt.Fprint(w, `{"data":{"accessToken":"dummy","refreshToken":"refresh","expiresIn":3600}}`)
			}
		case "/v2/plugin/login/account":
			if r.Header.Get("Authorization") != "Bearer dummy" {
				t.Error("account auth")
			}
			fmt.Fprint(w, `{"data":{"uid":"u","type":"ultimate"}}`)
		case "/v2/plugin/accounts":
			fmt.Fprint(w, `{"data":{"accounts":[{"uid":"u","type":"ultimate","enterpriseId":"team"},{"uid":"other","type":"ultimate","enterpriseId":"wrong"}]}}`)
		case "/v3/config":
			if r.Header.Get("X-Tenant-Id") != "team" || r.Header.Get("User-Agent") != "CLI/2.0.0 WorkBuddy/5.5.6" {
				t.Error("model headers")
			}
			fmt.Fprint(w, `{"data":{"agents":[{"name":"other","models":["hidden"]},{"name":"cli","models":["glm-test","fast"]}],"models":[{"id":"glm-test","supportsImages":true,"maxInputTokens":2000},{"id":"fast","name":"Fast","supportsImages":true}]}}`)
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	start, e := c.Start(context.Background(), "local")
	if e != nil || start.State != "remote" || start.ExpiresAt < time.Now().UnixMilli() {
		t.Fatal(start, e)
	}
	a, done, e := c.Poll(context.Background(), "remote", Config{})
	if e != nil || done {
		t.Fatal(e)
	}
	a, done, e = c.Poll(context.Background(), "remote", a)
	if e != nil || !done || a.EnterpriseID != "team" || a.ExpiresAt == 0 {
		t.Fatal(a, done, e)
	}
	m, e := c.Models(context.Background(), a)
	if e != nil || len(m) != 2 || m[0].Images || !m[1].Images || m[0].Context != 2000 {
		t.Fatal(m, e)
	}
}
func TestErrorsNoSecretEchoAndRetry(t *testing.T) {
	var n int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"code":11001,"msg":"dummy-secret"}`)
			return
		}
		fmt.Fprint(w, `{"data":{"accessToken":"new"}}`)
	}))
	defer s.Close()
	c := NewClient(s.URL, s.Client())
	a := Config{RefreshToken: "dummy"}
	if _, e := c.Refresh(context.Background(), a); e == nil || e.Error() != "WorkBuddy request failed (HTTP 401, code 11001)" {
		t.Fatal(e)
	}
	if _, e := c.Refresh(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	a.RefreshExpiresAt = 1
	if _, e := c.Refresh(context.Background(), a); e == nil {
		t.Fatal("expired refresh accepted")
	}
}
