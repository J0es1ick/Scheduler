package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/J0es1ick/Scheduler/internal/buildinfo"
)

type databaseHealth interface {
	PingContext(context.Context) error
}

func HealthHandler(database databaseHealth, monitor *Monitor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeWorkerHealth(w, http.StatusOK, map[string]any{"status": "alive", "build": buildinfo.Values()})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		ready := database != nil && database.PingContext(ctx) == nil
		checks := monitor.Checks()
		for _, passed := range checks {
			ready = ready && passed
		}
		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "not_ready"
		} else {
			for _, degraded := range monitor.DegradedChecks() {
				if degraded {
					state = "degraded"
					break
				}
			}
		}
		writeWorkerHealth(w, status, map[string]any{"status": state, "workers": checks})
	})
	return mux
}

func writeWorkerHealth(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
