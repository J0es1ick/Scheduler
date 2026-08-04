package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type databasePinger interface {
	PingContext(context.Context) error
}

type Health struct {
	database           databasePinger
	polling            atomic.Bool
	commandsConfigured atomic.Bool
}

func NewHealth(database databasePinger) *Health {
	return &Health{database: database}
}

func (h *Health) SetPolling(value bool) {
	h.polling.Store(value)
}

func (h *Health) SetCommandsConfigured(value bool) {
	h.commandsConfigured.Store(value)
}

func (h *Health) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeHealthJSON(w, http.StatusOK, map[string]any{"status": "alive"})
	})
	mux.HandleFunc("GET /ready", h.ready)
	return mux
}

func (h *Health) ready(w http.ResponseWriter, request *http.Request) {
	checks := map[string]bool{
		"polling":             h.polling.Load(),
		"commands_configured": h.commandsConfigured.Load(),
		"database":            false,
	}
	if h.database != nil {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		checks["database"] = h.database.PingContext(ctx) == nil
		cancel()
	}

	status := http.StatusOK
	for _, passed := range checks {
		if !passed {
			status = http.StatusServiceUnavailable
			break
		}
	}
	writeHealthJSON(w, status, map[string]any{
		"status": map[bool]string{true: "ready", false: "not_ready"}[status == http.StatusOK],
		"checks": checks,
	})
}

func writeHealthJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
