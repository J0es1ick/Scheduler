package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubPinger struct{ err error }

type stubWorkers map[string]bool

func (w stubWorkers) Checks() map[string]bool { return w }

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
