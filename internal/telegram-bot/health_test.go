package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubPinger struct{ err error }

type stubWorkers map[string]bool

func (w stubWorkers) Checks() map[string]bool { return w }

type degradedWorkers struct {
	checks   map[string]bool
	degraded map[string]bool
}

func (w degradedWorkers) Checks() map[string]bool         { return w.checks }
func (w degradedWorkers) DegradedChecks() map[string]bool { return w.degraded }

func (p stubPinger) PingContext(context.Context) error { return p.err }

func TestHealthReadinessRequiresAllDependencies(t *testing.T) {
	health := NewHealth(stubPinger{})
	health.SetPolling(true)
	recorder := httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status before command setup = %d", recorder.Code)
	}

	health.SetCommandsConfigured(true)
	recorder = httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthReadinessChecksWorkers(t *testing.T) {
	workers := stubWorkers{"parser": true, "reminder": false}
	health := NewHealth(stubPinger{}, workers)
	health.SetPolling(true)
	health.SetCommandsConfigured(true)
	recorder := httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status with stopped worker = %d", recorder.Code)
	}
	workers["reminder"] = true
	recorder = httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status with ready workers = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthReadinessChecksDatabase(t *testing.T) {
	health := NewHealth(stubPinger{err: errors.New("down")})
	health.SetPolling(true)
	health.SetCommandsConfigured(true)
	recorder := httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status with unavailable database = %d", recorder.Code)
	}
}

func TestHealthReportsExternalDegradationAsReady(t *testing.T) {
	health := NewHealth(stubPinger{}, degradedWorkers{
		checks:   map[string]bool{"parser": true},
		degraded: map[string]bool{"parser": true},
	})
	health.SetPolling(true)
	health.SetCommandsConfigured(true)
	recorder := httptest.NewRecorder()
	health.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("degraded worker readiness status=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"status":"degraded"`) {
		t.Fatalf("degraded readiness was not reported: %s", body)
	}
	if body := recorder.Body.String(); strings.Contains(body, `"checks"`) || strings.Contains(body, `"degraded":`) {
		t.Fatalf("public readiness exposed internal check names: %s", body)
	}
}
