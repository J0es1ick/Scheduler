package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"name":"first"} {"name":"second"}`))
	recorder := httptest.NewRecorder()
	var payload struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(recorder, request, &payload); err == nil {
		t.Fatal("trailing JSON value must be rejected")
	}
}

func TestRequestIPTrustsForwardedHeaderOnlyFromConfiguredProxy(t *testing.T) {
	proxies, err := parseTrustedProxies("127.0.0.1/32")
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	server := &Server{trustedProxies: proxies}

	trusted := httptest.NewRequest(http.MethodGet, "/", nil)
	trusted.RemoteAddr = "127.0.0.1:44000"
	trusted.Header.Set("X-Forwarded-For", "203.0.113.25, 127.0.0.1")
	if got := server.requestIP(trusted); got != "203.0.113.25" {
		t.Fatalf("trusted proxy client IP = %q", got)
	}

	untrusted := httptest.NewRequest(http.MethodGet, "/", nil)
	untrusted.RemoteAddr = "198.51.100.14:44000"
	untrusted.Header.Set("X-Forwarded-For", "203.0.113.25")
	if got := server.requestIP(untrusted); got != "198.51.100.14" {
		t.Fatalf("untrusted proxy client IP = %q", got)
	}
}

func TestRequestIPIgnoresSpoofedLeftmostForwardedAddress(t *testing.T) {
	proxies, err := parseTrustedProxies("127.0.0.1/32")
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	server := &Server{trustedProxies: proxies}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:44000"
	request.Header.Set("X-Forwarded-For", "203.0.113.25, 198.51.100.14")

	if got := server.requestIP(request); got != "198.51.100.14" {
		t.Fatalf("forwarded client IP = %q, want nearest untrusted hop", got)
	}
}

func TestRequestIPWalksKnownProxyChainRightToLeft(t *testing.T) {
	proxies, err := parseTrustedProxies("127.0.0.1/32,198.51.100.0/24")
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	server := &Server{trustedProxies: proxies}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:44000"
	request.Header.Set("X-Forwarded-For", "203.0.113.25, 198.51.100.14")

	if got := server.requestIP(request); got != "203.0.113.25" {
		t.Fatalf("forwarded client IP = %q, want original client", got)
	}
}

func TestRecoverPanicDoesNotOverwriteStartedResponse(t *testing.T) {
	server := &Server{}
	handler := server.requestContext(server.requestLog(server.recoverPanic(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("boom")
	}))))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if recorder.Code != http.StatusAccepted || recorder.Body.String() != "partial" {
		t.Fatalf("started response was overwritten: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestAccessKeyMustBeExplicitlyEnabled(t *testing.T) {
	auth := NewAuthManager("bot-token", "configured-but-disabled", false, true)
	if auth.AccessKeyEnabled() {
		t.Fatal("access-key login unexpectedly enabled")
	}
	if _, err := auth.LoginWithAccessKey("configured-but-disabled"); err != ErrForbidden {
		t.Fatalf("disabled access-key login error = %v", err)
	}
}
