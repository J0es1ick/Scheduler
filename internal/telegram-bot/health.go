package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/J0es1ick/Scheduler/internal/buildinfo"
)

type databasePinger interface {
	PingContext(context.Context) error
}

type workerReadiness interface {
	Checks() map[string]bool
}

type Health struct {
	database           databasePinger
	polling            atomic.Bool
	commandsConfigured atomic.Bool
	workers            workerReadiness
}

func NewHealth(database databasePinger, workers ...workerReadiness) *Health {
	health := &Health{database: database}
	if len(workers) > 0 {
		health.workers = workers[0]
	}
	return health
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
		writeHealthJSON(w, http.StatusOK, map[string]any{
			"status": "alive",
			"build":  buildinfo.Values(),
		})
	})
	mux.HandleFunc("GET /ready", h.ready)
	mux.HandleFunc("GET /metrics", h.metrics)
	return mux
}

func (h *Health) metrics(w http.ResponseWriter, _ *http.Request) {
	active, waiting, rejected := HandlerLoad()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, `# TYPE scheduler_telegram_handlers gauge
scheduler_telegram_handlers{state="active"} %d
scheduler_telegram_handlers{state="waiting"} %d
# TYPE scheduler_telegram_handler_rejections_total counter
scheduler_telegram_handler_rejections_total %d
`, active, waiting, rejected)
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
	if h.workers != nil {
		for name, ready := range h.workers.Checks() {
			checks["worker_"+name] = ready
		}
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
