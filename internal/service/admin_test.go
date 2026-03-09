package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProxyStatusReportsHealthyServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	svc := newAdminServiceForProxyStatusTest(server.Listener.Addr().String())
	req := httptest.NewRequest(http.MethodGet, "http://docs.example/api/admin/proxy-status", nil)
	req.Host = "docs.example"

	status := svc.GetProxyStatus(req)

	if !status.Running {
		t.Fatalf("expected proxy status to report running")
	}
	if status.Address != "docs.example" {
		t.Fatalf("expected display address docs.example, got %q", status.Address)
	}
	if status.Port != 80 {
		t.Fatalf("expected display port 80, got %d", status.Port)
	}
}

func TestGetProxyStatusReportsUnhealthyServer(t *testing.T) {
	svc := newAdminServiceForProxyStatusTest("127.0.0.1:1")
	req := httptest.NewRequest(http.MethodGet, "http://docs.example/api/admin/proxy-status", nil)
	req.Host = "docs.example:8443"
	req.Header.Set("X-Forwarded-Proto", "https")

	status := svc.GetProxyStatus(req)

	if status.Running {
		t.Fatalf("expected proxy status to report not running")
	}
	if status.Port != 8443 {
		t.Fatalf("expected display port 8443, got %d", status.Port)
	}
}

func newAdminServiceForProxyStatusTest(serverAddr string) *AdminService {
	return NewAdminService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		serverAddr,
		nil,
		nil,
		nil,
	)
}
