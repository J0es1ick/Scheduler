package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/servicelogs"
)

func TestRequestLogClassifiesHTTPFailures(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for status, level := range map[int]string{200: "INFO", 403: "WARN", 500: "ERROR"} {
		output.Reset()
		server := &Server{}
		handler := server.requestLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/synthetic", nil))
		var entry map[string]any
		if err := json.Unmarshal(output.Bytes(), &entry); err != nil || entry["level"] != level || entry["module"] != "http" {
			t.Fatalf("HTTP %d missing from %s filter: %s (%v)", status, level, output.String(), err)
		}
	}
}

type fakeServiceLogs struct {
	err   error
	calls int
}

func (f *fakeServiceLogs) Read(context.Context, url.Values) (servicelogs.Page, error) {
	f.calls++
	return servicelogs.Page{Entries: []servicelogs.Entry{}, Components: []servicelogs.Component{}, Modules: []string{}, Warnings: []string{}}, f.err
}

func TestServiceLogsAuthorizationAndOutage(t *testing.T) {
	for _, role := range []string{"read_only", "support", "editor", "reviewer", "operator", "owner"} {
		t.Run(role, func(t *testing.T) {
			reader := &fakeServiceLogs{}
			auth := NewAuthManager("bot", "test-access-key", true, false)
			server, err := NewServer(nil, auth, nil, ServerOptions{ServiceLogs: reader})
			if err != nil {
				t.Fatal(err)
			}
			issued := httptest.NewRecorder()
			_, err = auth.IssueSession(issued, AdminIdentity{ID: "local-admin", Role: role, AuthMethod: "access_key"})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/api/service-logs", nil)
			req.AddCookie(issued.Result().Cookies()[0])
			rr := httptest.NewRecorder()
			server.Handler().ServeHTTP(rr, req)
			allowed := role == "operator" || role == "owner"
			if allowed {
				if rr.Code != 200 || reader.calls != 1 || rr.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("allowed role rejected: %d %s", rr.Code, rr.Body.String())
				}
			} else if rr.Code != 403 || reader.calls != 0 {
				t.Fatalf("role %s received logs: %d", role, rr.Code)
			}
			if allowed {
				reader.err = errors.New("synthetic reader outage")
				rr = httptest.NewRecorder()
				server.Handler().ServeHTTP(rr, req)
				if rr.Code != 503 {
					t.Fatalf("outage disguised as empty data: %d", rr.Code)
				}
			}
		})
	}
}
