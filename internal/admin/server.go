package admin

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/adminui"
	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
	"github.com/google/uuid"
)

const (
	requestIDHeader       = "X-Request-ID"
	internalAdminIDHeader = "X-Scheduler-Admin-ID"
)

type ServerOptions struct {
	MetricsToken      string
	TrustedProxyCIDRs string
	ConnectorHandler  http.Handler
	ManagedParsers    []managed.Manifest
	WorkerReadiness   interface{ Checks() map[string]bool }
}

type Server struct {
	store           *Store
	auth            *AuthManager
	parser          *service.ParserService
	logins          *loginGuard
	globalLogins    *loginGuard
	metricsToken    string
	trustedProxies  []*net.IPNet
	managedParsers  map[string]managed.Manifest
	workerReadiness interface{ Checks() map[string]bool }

	runningMu sync.RWMutex
	running   map[string]bool
	handler   http.Handler
}

func NewServer(store *Store, auth *AuthManager, parser *service.ParserService, options ...ServerOptions) (*Server, error) {
	var opts ServerOptions
	if len(options) > 0 {
		opts = options[0]
	}
	trustedProxies, err := parseTrustedProxies(opts.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	catalog := make(map[string]managed.Manifest, len(opts.ManagedParsers))
	for _, item := range opts.ManagedParsers {
		item = managed.NormalizeManifest(item)
		if err := managed.ValidateManifest(item); err != nil {
			return nil, fmt.Errorf("managed parser catalog %q: %w", item.ParserID, err)
		}
		if _, exists := catalog[item.ParserID]; exists {
			return nil, fmt.Errorf("managed parser %q is registered twice", item.ParserID)
		}
		catalog[item.ParserID] = item
	}
	server := &Server{
		store:           store,
		auth:            auth,
		parser:          parser,
		logins:          newLoginGuard(),
		globalLogins:    newLoginGuard(50),
		metricsToken:    strings.TrimSpace(opts.MetricsToken),
		trustedProxies:  trustedProxies,
		managedParsers:  catalog,
		workerReadiness: opts.WorkerReadiness,
		running:         make(map[string]bool),
	}
	assets, err := adminui.Files()
	if err != nil {
		return nil, fmt.Errorf("admin UI assets: %w", err)
	}
	index, err := adminui.Index()
	if err != nil {
		return nil, fmt.Errorf("admin UI index: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.handleHealth)
	mux.HandleFunc("GET /api/ready", server.handleReady)
	mux.HandleFunc("GET /metrics", server.handleMetrics)
	mux.HandleFunc("GET /api/auth/config", server.handleAuthConfig)
	mux.HandleFunc("POST /api/auth/access-key", server.handleAccessKeyLogin)
	mux.HandleFunc("POST /api/auth/telegram", server.handleTelegramLogin)
	if opts.ConnectorHandler != nil {
		mux.Handle("/api/v1/connector-spec", opts.ConnectorHandler)
		mux.Handle("/api/v1/connectors/", opts.ConnectorHandler)
	}
	server.protected(mux, "GET /api/auth/me", server.handleMe)
	server.protected(mux, "POST /api/auth/logout", server.handleLogout)
	server.protected(mux, "POST /api/client-errors", server.handleClientError)
	server.protected(mux, "GET /api/dashboard", server.handleDashboard)
	server.protected(mux, "GET /api/sources", server.handleSources)
	server.protected(mux, "PATCH /api/sources/{id}", server.handleUpdateSource)
	server.protected(mux, "DELETE /api/sources/{id}", server.handleDeleteSource)
	server.protected(mux, "POST /api/sources/{id}/restore", server.handleRestoreSource)
	server.protected(mux, "POST /api/sources/{id}/sync", server.handleSyncSource)
	server.protected(mux, "POST /api/sources/{id}/rollback", server.handleRollbackSource)
	server.protected(mux, "GET /api/connectors", server.handleConnectors)
	server.protected(mux, "GET /api/connectors/catalog", server.handleConnectorCatalog)
	server.protected(mux, "POST /api/connectors", server.handleCreateConnector)
	server.protected(mux, "PATCH /api/connectors/{id}", server.handleUpdateConnector)
	server.protected(mux, "POST /api/connectors/{id}/rotate-key", server.handleRotateConnectorKey)
	server.protected(mux, "GET /api/connectors/{id}/runs", server.handleConnectorRuns)
	server.protected(mux, "GET /api/parser-snapshots", server.handleParserSnapshots)
	server.protected(mux, "GET /api/parser-snapshots/{id}/preview", server.handleParserSnapshotPreview)
	server.protected(mux, "GET /api/parser-snapshots/{id}/schedule", server.handleParserSnapshotSchedule)
	server.protected(mux, "POST /api/parser-snapshots/{id}/publish", server.handlePublishSnapshot)
	server.protected(mux, "POST /api/parser-snapshots/{id}/reject", server.handleRejectSnapshot)
	server.protected(mux, "GET /api/operations", server.handleOperations)
	server.protected(mux, "GET /api/logs", server.handleLogs)
	server.protected(mux, "GET /api/universities", server.handleUniversities)
	server.protected(mux, "GET /api/groups", server.handleGroups)
	server.protected(mux, "GET /api/lessons", server.handleLessons)
	server.protected(mux, "GET /api/editor/schedule", server.handleEditorSchedule)
	server.protected(mux, "POST /api/editor/lessons", server.handleCreateEditorLesson)
	server.protected(mux, "PUT /api/editor/lessons/{id}", server.handleUpdateEditorLesson)
	server.protected(mux, "DELETE /api/editor/lessons/{id}", server.handleDeleteEditorLesson)
	server.protected(mux, "POST /api/editor/lessons/{id}/restore", server.handleRestoreEditorLesson)
	server.protected(mux, "GET /api/support-requests", server.handleSupportRequests)
	server.protected(mux, "PATCH /api/support-requests/{id}", server.handleResolveSupportRequest)
	server.protected(mux, "GET /api/users", server.handleUsers)
	server.protected(mux, "PATCH /api/users/{id}", server.handleUpdateUser)
	server.protected(mux, "GET /api/audit", server.handleAudit)
	mux.Handle("/", spaHandler(assets, index))

	server.handler = server.requestContext(server.requestLog(server.recoverPanic(server.securityHeaders(mux))))
	return server, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) protected(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	protectedHandler := http.Handler(handler)
	requiredRole := roleForPattern(pattern)
	nextByRole := protectedHandler
	protectedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := identityFromContext(r.Context())
		if !roleAllows(identity.Role, requiredRole) {
			writeAPIError(w, http.StatusForbidden, "Недостаточно прав для этого действия")
			return
		}
		nextByRole.ServeHTTP(w, r)
	})
	if isMutationPattern(pattern) && pattern != "POST /api/client-errors" {
		next := protectedHandler
		protectedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := identityFromContext(r.Context())
			details := map[string]any{
				"method":     r.Method,
				"path":       r.URL.Path,
				"request_id": r.Header.Get(requestIDHeader),
			}
			if err := s.store.WriteAudit(
				r.Context(), identity, "mutation_requested", "http_request", pattern, details, s.requestIP(r),
			); err != nil {
				slog.Error("admin mutation audit precondition failed", "pattern", pattern, "admin_id", identity.ID, "err", err)
				writeAPIError(w, http.StatusServiceUnavailable, "Не удалось зафиксировать административное действие")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle(pattern, s.auth.Require(s.store, protectedHandler))
}

func roleForPattern(pattern string) string {
	switch {
	case strings.HasPrefix(pattern, "GET /api/users"):
		return "owner"
	case strings.HasPrefix(pattern, "GET /api/audit"):
		return "operator"
	case strings.HasPrefix(pattern, "GET /api/support-requests"):
		return "support"
	case strings.HasPrefix(pattern, "GET /api/parser-snapshots"):
		return "reviewer"
	case strings.HasPrefix(pattern, "GET /api/connectors"):
		return "operator"
	}
	if strings.HasPrefix(pattern, "GET ") || strings.HasPrefix(pattern, "HEAD ") {
		return "read_only"
	}
	switch {
	case strings.HasPrefix(pattern, "PATCH /api/users/"):
		return "owner"
	case strings.Contains(pattern, "/connectors"), strings.Contains(pattern, "/sources"):
		return "operator"
	case strings.Contains(pattern, "/parser-snapshots/"):
		return "reviewer"
	case strings.Contains(pattern, "/editor/"):
		return "editor"
	case strings.Contains(pattern, "/support-requests/"):
		return "support"
	default:
		return "operator"
	}
}

func roleAllows(actual, required string) bool {
	if actual == "owner" {
		return true
	}
	if required == "read_only" {
		return actual != "" && actual != "none"
	}
	permissions := map[string]map[string]bool{
		"support":  {"support": true},
		"editor":   {"editor": true},
		"reviewer": {"reviewer": true, "editor": true},
		"operator": {"operator": true, "reviewer": true, "editor": true, "support": true},
	}
	return permissions[actual][required]
}

func isMutationPattern(pattern string) bool {
	return !strings.HasPrefix(pattern, http.MethodGet+" ") &&
		!strings.HasPrefix(pattern, http.MethodHead+" ") &&
		!strings.HasPrefix(pattern, http.MethodOptions+" ")
}

func (s *Server) writeAudit(
	ctx context.Context,
	identity AdminIdentity,
	action, objectType, objectID string,
	details any,
	ipAddress string,
) {
	if err := s.store.WriteAudit(ctx, identity, action, objectType, objectID, details, ipAddress); err != nil {
		slog.Error(
			"admin audit write failed",
			"action", action,
			"object_type", objectType,
			"object_id", objectID,
			"admin_id", identity.ID,
			"err", err,
		)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now(),
		"build":  buildinfo.Values(),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return
	}
	if s.workerReadiness != nil {
		checks := s.workerReadiness.Checks()
		for _, ready := range checks {
			if !ready {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"status": "not_ready",
					"checks": checks,
				})
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metricsToken == "" || !constantTimeTokenEqual(bearerToken(r), s.metricsToken) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	operations, err := s.store.OperationalHealth(ctx)
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	databaseStats := s.store.db.Stats()
	status := 1
	if operations.Status != "healthy" {
		status = 0
	}
	reminderCursorPending := 0
	if operations.ReminderWorker.Cursor != "" {
		reminderCursorPending = 1
	}
	_, _ = fmt.Fprintf(w, `# HELP scheduler_health Overall scheduler health (1 healthy, 0 degraded).
# TYPE scheduler_health gauge
scheduler_health %d
# TYPE scheduler_sources gauge
scheduler_sources{state="healthy"} %d
scheduler_sources{state="running"} %d
scheduler_sources{state="stale"} %d
scheduler_sources{state="error"} %d
scheduler_sources{state="quarantined"} %d
scheduler_sources{state="disabled"} %d
# TYPE scheduler_notification_queue gauge
scheduler_notification_queue{queue="schedule",status="pending"} %d
scheduler_notification_queue{queue="schedule",status="failed"} %d
scheduler_notification_queue{queue="outbox",status="pending"} %d
scheduler_notification_queue{queue="outbox",status="failed"} %d
# TYPE scheduler_connector_queue gauge
scheduler_connector_queue{status="pending"} %d
scheduler_connector_queue{status="failed"} %d
# TYPE scheduler_database_connections gauge
scheduler_database_connections{state="open"} %d
scheduler_database_connections{state="in_use"} %d
scheduler_database_connections{state="idle"} %d
# TYPE scheduler_oldest_pending_seconds gauge
scheduler_oldest_pending_seconds %d
# TYPE scheduler_reminder_worker_last_run_timestamp_seconds gauge
scheduler_reminder_worker_last_run_timestamp_seconds %d
# TYPE scheduler_reminder_worker_last_full_cycle_timestamp_seconds gauge
scheduler_reminder_worker_last_full_cycle_timestamp_seconds %d
# TYPE scheduler_reminder_worker_last_duration_seconds gauge
scheduler_reminder_worker_last_duration_seconds %.3f
# TYPE scheduler_reminder_worker_last_processed gauge
scheduler_reminder_worker_last_processed %d
# TYPE scheduler_reminder_worker_last_failures gauge
scheduler_reminder_worker_last_failures %d
# TYPE scheduler_reminder_worker_cursor_pending gauge
scheduler_reminder_worker_cursor_pending %d
`,
		status,
		operations.SourcesHealthy,
		operations.SourcesRunning,
		operations.SourcesStale,
		operations.SourcesError,
		operations.SourcesQuarantined,
		operations.SourcesDisabled,
		operations.PendingNotifications,
		operations.FailedNotifications,
		operations.PendingOutbox,
		operations.FailedOutbox,
		operations.PendingConnectorRuns,
		operations.FailedConnectorRuns,
		databaseStats.OpenConnections,
		databaseStats.InUse,
		databaseStats.Idle,
		operations.OldestPendingSeconds,
		unixTimestamp(operations.ReminderWorker.LastFinishedAt),
		unixTimestamp(operations.ReminderWorker.LastFullCycleAt),
		float64(operations.ReminderWorker.LastDurationMS)/1000,
		operations.ReminderWorker.LastProcessed,
		operations.ReminderWorker.LastFailures,
		reminderCursorPending,
	)
}

func unixTimestamp(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < 7 || !strings.EqualFold(value[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func constantTimeTokenEqual(actual, expected string) bool {
	actualHash := sha256.Sum256([]byte(actual))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(actualHash[:], expectedHash[:]) == 1
}

func (s *Server) handleAccessKeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.auth.AccessKeyEnabled() {
		http.NotFound(w, r)
		return
	}
	ip := s.requestIP(r)
	now := time.Now()
	if allowed, retryAfter := s.logins.allowed(ip, now); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
		writeAPIError(w, http.StatusTooManyRequests, "Слишком много попыток входа. Повторите позже.")
		return
	}
	if allowed, retryAfter := s.globalLogins.allowed("access-key", now); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
		writeAPIError(w, http.StatusTooManyRequests, "Вход по аварийному ключу временно заблокирован.")
		return
	}
	var request struct {
		AccessKey string `json:"access_key"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	identity, err := s.auth.LoginWithAccessKey(request.AccessKey)
	if err != nil {
		now = time.Now()
		s.logins.failed(ip, now)
		s.globalLogins.failed("access-key", now)
		writeAPIError(w, http.StatusUnauthorized, "Неверный ключ администратора")
		return
	}
	s.logins.succeeded(ip)
	s.globalLogins.succeeded("access-key")
	identity, err = s.auth.IssueSession(w, identity)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать сессию")
		return
	}
	s.writeAudit(r.Context(), identity, "login", "session", "", map[string]any{"method": "access_key"}, ip)
	writeJSON(w, http.StatusOK, map[string]any{"user": identity})
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"access_key_enabled": s.auth.AccessKeyEnabled(),
	})
}

func (s *Server) handleClientError(w http.ResponseWriter, r *http.Request) {
	var report struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
		URL     string `json:"url"`
	}
	if err := decodeJSON(w, r, &report); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный отчёт об ошибке")
		return
	}
	identity := identityFromContext(r.Context())
	slog.Error("admin client error",
		"admin_id", identity.ID,
		"message", truncateReportField(report.Message, 1000),
		"stack", truncateReportField(report.Stack, 4000),
		"url", truncateReportField(report.URL, 500),
	)
	w.WriteHeader(http.StatusNoContent)
}

func truncateReportField(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		InitData string `json:"init_data"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	identity, err := s.auth.LoginWithTelegram(r.Context(), s.store, request.InitData)
	if err != nil {
		writeAPIError(w, http.StatusForbidden, "Этот Telegram-пользователь не является администратором")
		return
	}
	identity, err = s.auth.IssueSession(w, identity)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать сессию")
		return
	}
	s.writeAudit(r.Context(), identity, "login", "session", "", map[string]any{"method": "telegram"}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"user": identity})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"user":               identityFromContext(r.Context()),
		"access_key_enabled": s.auth.AccessKeyEnabled(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	s.writeAudit(r.Context(), identity, "logout", "session", "", map[string]any{}, s.requestIP(r))
	s.auth.Logout(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := s.store.Dashboard(r.Context())
	if err != nil {
		slog.Error("admin dashboard failed", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить обзор")
		return
	}
	s.enrichRunning(dashboard.Sources)
	writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.store.Sources(r.Context(), true)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить источники")
		return
	}
	s.enrichRunning(sources)
	writeJSON(w, http.StatusOK, map[string]any{"items": sources})
}

func (s *Server) handleRestoreSource(w http.ResponseWriter, r *http.Request) {
	sourceID := strings.TrimSpace(r.PathValue("id"))
	if sourceID == "" {
		writeAPIError(w, http.StatusBadRequest, "Источник не указан")
		return
	}
	lifecycle, err := s.store.RestoreSource(r.Context(), sourceID)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "Архивный источник не найден")
		case errors.Is(err, ErrSourceBusy):
			writeAPIError(w, http.StatusConflict, "Источник сейчас обновляется")
		case errors.Is(err, repository.ErrActiveSourceConflict):
			writeAPIError(w, http.StatusConflict, "У этого вуза уже есть активный источник. Сначала приостановите его")
		default:
			slog.Error("admin restore source failed", "source", sourceID, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "Не удалось восстановить источник")
		}
		return
	}
	identity := identityFromContext(r.Context())
	s.writeAudit(r.Context(), identity, "restore_source", "data_source", sourceID,
		map[string]any{"lifecycle_status": lifecycle, "enabled": false}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "restored", "lifecycle_status": lifecycle})
}

func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request) {
	operations, err := s.store.OperationalHealth(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось проверить состояние сервиса")
		return
	}
	writeJSON(w, http.StatusOK, operations)
}

func (s *Server) handleParserSnapshots(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ParserSnapshots(
		r.Context(),
		strings.TrimSpace(r.URL.Query().Get("source")),
		strings.TrimSpace(r.URL.Query().Get("status")),
		queryInt(r, "limit", 50),
	)
	if err != nil {
		slog.Error("admin list parser snapshots failed", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить снимки")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleParserSnapshotPreview(w http.ResponseWriter, r *http.Request) {
	preview, err := s.store.ParserSnapshotPreview(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "Снимок не найден")
		return
	}
	if err != nil {
		slog.Error("admin load parser snapshot preview failed", "snapshot", r.PathValue("id"), "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось подготовить сравнение снимка")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleParserSnapshotSchedule(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group"))
	if groupID == "" {
		writeAPIError(w, http.StatusBadRequest, "Укажите учебную группу")
		return
	}
	comparison, err := s.store.ParserSnapshotSchedule(
		r.Context(),
		r.PathValue("id"),
		groupID,
	)
	if errors.Is(err, ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "Снимок или группа не найдены")
		return
	}
	if err != nil {
		slog.Error(
			"admin load parser snapshot schedule failed",
			"snapshot", r.PathValue("id"),
			"group", groupID,
			"err", err,
		)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить расписание группы")
		return
	}
	writeJSON(w, http.StatusOK, comparison)
}

func (s *Server) handlePublishSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID := r.PathValue("id")
	var request struct {
		ReviewNote string `json:"review_note"`
	}
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
			return
		}
	}
	identity := identityFromContext(r.Context())
	snapshot, err := s.parser.PublishSnapshot(
		r.Context(), snapshotID, identity.ID, strings.TrimSpace(request.ReviewNote),
	)
	if errors.Is(err, service.ErrDataSourceBusy) {
		writeAPIError(w, http.StatusConflict, "Источник сейчас обновляется")
		return
	}
	if errors.Is(err, sql.ErrNoRows) || snapshot == nil && err == nil {
		writeAPIError(w, http.StatusNotFound, "Снимок не найден")
		return
	}
	if err != nil {
		slog.Error("admin publish parser snapshot failed", "snapshot", snapshotID, "err", err)
		writeAPIError(w, http.StatusConflict, "Снимок нельзя опубликовать: "+err.Error())
		return
	}
	s.writeAudit(r.Context(), identity, "publish_parser_snapshot", "parser_snapshot", snapshotID,
		map[string]any{
			"source_id":   snapshot.DataSourceID,
			"groups":      snapshot.GroupCount,
			"lessons":     snapshot.LessonCount,
			"review_note": request.ReviewNote,
		}, s.requestIP(r))
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleRejectSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID := r.PathValue("id")
	var request struct {
		ReviewNote string `json:"review_note"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	request.ReviewNote = strings.TrimSpace(request.ReviewNote)
	if request.ReviewNote == "" {
		writeAPIError(w, http.StatusBadRequest, "Укажите причину отклонения")
		return
	}
	identity := identityFromContext(r.Context())
	if err := s.parser.RejectSnapshot(r.Context(), snapshotID, identity.ID, request.ReviewNote); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "Снимок не найден или уже обработан")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось отклонить снимок")
		return
	}
	s.writeAudit(r.Context(), identity, "reject_parser_snapshot", "parser_snapshot", snapshotID,
		map[string]any{"review_note": request.ReviewNote}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

func (s *Server) handleRollbackSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	identity := identityFromContext(r.Context())
	snapshot, err := s.parser.RollbackSource(
		r.Context(), sourceID, identity.ID, "Откат из административной панели",
	)
	if errors.Is(err, service.ErrDataSourceBusy) {
		writeAPIError(w, http.StatusConflict, "Источник сейчас обновляется")
		return
	}
	if err != nil {
		slog.Error("admin rollback source failed", "source", sourceID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось восстановить предыдущий снимок")
		return
	}
	if snapshot == nil {
		writeAPIError(w, http.StatusNotFound, "Предыдущий опубликованный снимок отсутствует")
		return
	}
	s.writeAudit(r.Context(), identity, "rollback_parser_snapshot", "data_source", sourceID,
		map[string]any{"snapshot_id": snapshot.ID}, s.requestIP(r))
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	var request struct {
		UpdateInterval *int  `json:"update_interval"`
		IsEnabled      *bool `json:"is_enabled"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	if request.UpdateInterval == nil && request.IsEnabled == nil {
		writeAPIError(w, http.StatusBadRequest, "Не указаны настройки источника")
		return
	}
	if request.UpdateInterval != nil && (*request.UpdateInterval < 300 || *request.UpdateInterval > 604800) {
		writeAPIError(w, http.StatusBadRequest, "Интервал должен быть от 5 минут до 7 дней")
		return
	}
	if err := s.store.UpdateSourceSettings(
		r.Context(), sourceID, request.UpdateInterval, request.IsEnabled,
	); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "Источник не найден")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить интервал")
		return
	}
	identity := identityFromContext(r.Context())
	details := map[string]any{}
	action := "update_interval"
	if request.UpdateInterval != nil {
		details["update_interval"] = *request.UpdateInterval
	}
	if request.IsEnabled != nil {
		details["is_enabled"] = *request.IsEnabled
		if *request.IsEnabled {
			action = "enable_source"
		} else {
			action = "disable_source"
		}
	}
	s.writeAudit(r.Context(), identity, action, "data_source", sourceID, details, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	sourceID := strings.TrimSpace(r.PathValue("id"))
	if sourceID == "" {
		writeAPIError(w, http.StatusBadRequest, "Источник не указан")
		return
	}
	if err := s.store.DeleteSource(r.Context(), sourceID); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "Источник не найден")
		case errors.Is(err, ErrSourceBusy):
			writeAPIError(w, http.StatusConflict, "Источник сейчас обновляется")
		default:
			slog.Error("admin delete source failed", "source", sourceID, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "Не удалось удалить источник")
		}
		return
	}
	identity := identityFromContext(r.Context())
	s.writeAudit(r.Context(), identity, "delete_source", "data_source", sourceID,
		map[string]any{"published_schedule_preserved": true}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (s *Server) handleSyncSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	if sourceID == "" {
		writeAPIError(w, http.StatusBadRequest, "Источник не указан")
		return
	}
	_, err := s.store.SourceEnabled(r.Context(), sourceID)
	if errors.Is(err, ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "Источник не найден")
		return
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось проверить источник")
		return
	}
	if !s.beginSync(sourceID) {
		writeAPIError(w, http.StatusConflict, "Этот источник уже обновляется")
		return
	}
	identity := identityFromContext(r.Context())
	ipAddress := s.requestIP(r)
	s.writeAudit(r.Context(), identity, "sync_requested", "data_source", sourceID, map[string]any{}, ipAddress)
	go func() {
		defer s.finishSync(sourceID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		records, err := s.parser.RunDataSourceManual(ctx, sourceID)
		details := map[string]any{"records": records}
		action := "sync_completed"
		if err != nil {
			action = "sync_failed"
			details["error"] = err.Error()
			slog.Error("admin manual sync failed", "source", sourceID, "err", err)
		} else {
			slog.Info("admin manual sync complete", "source", sourceID, "records", records)
		}
		s.writeAudit(context.Background(), identity, action, "data_source", sourceID, details, ipAddress)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "source_id": sourceID})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.store.Logs(r.Context(), queryInt(r, "limit", 100), r.URL.Query().Get("source"), r.URL.Query().Get("status"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить историю")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs})
}

func (s *Server) handleUniversities(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Universities(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить университеты")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.Groups(r.Context(), queryInt(r, "page", 1), queryInt(r, "page_size", 30),
		r.URL.Query().Get("university"), strings.TrimSpace(r.URL.Query().Get("q")),
		r.URL.Query().Get("selector") == "true")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить группы")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLessons(w http.ResponseWriter, r *http.Request) {
	result, err := s.store.Lessons(r.Context(), queryInt(r, "page", 1), queryInt(r, "page_size", 30),
		r.URL.Query().Get("university"), r.URL.Query().Get("group"), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить занятия")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.Users(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), queryInt(r, "limit", 100))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить пользователей")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": users})
}

func (s *Server) handleSupportRequests(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.SupportRequests(
		r.Context(),
		strings.TrimSpace(r.URL.Query().Get("status")),
		strings.TrimSpace(r.URL.Query().Get("type")),
		strings.TrimSpace(r.URL.Query().Get("q")),
		queryInt(r, "limit", 200),
	)
	if err != nil {
		slog.Error("admin list support requests failed", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить обращения")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleResolveSupportRequest(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")
	var request struct {
		Status     string `json:"status"`
		ReviewNote string `json:"review_note"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	request.Status = strings.TrimSpace(request.Status)
	request.ReviewNote = strings.TrimSpace(request.ReviewNote)
	if request.Status != "approved" && request.Status != "rejected" {
		writeAPIError(w, http.StatusBadRequest, "Допустимо принять или отклонить обращение")
		return
	}
	if len([]rune(request.ReviewNote)) > 1000 {
		writeAPIError(w, http.StatusBadRequest, "Комментарий не должен превышать 1000 символов")
		return
	}
	if request.Status == "rejected" && request.ReviewNote == "" {
		writeAPIError(w, http.StatusBadRequest, "Укажите причину отклонения")
		return
	}

	identity := identityFromContext(r.Context())
	err := s.store.ResolveSupportRequest(r.Context(), requestID, request.Status, request.ReviewNote, identity.ID)
	if errors.Is(err, ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "Обращение не найдено")
		return
	}
	if errors.Is(err, ErrConflict) {
		writeAPIError(w, http.StatusConflict, "Обращение уже рассмотрено")
		return
	}
	if err != nil {
		slog.Error("admin resolve support request failed", "request_id", requestID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось обработать обращение")
		return
	}
	s.writeAudit(r.Context(), identity, "resolve_support_request", "support_request", requestID,
		map[string]any{"status": request.Status, "review_note": request.ReviewNote}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": request.Status})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	var request struct {
		IsAdmin   *bool  `json:"is_admin"`
		AdminRole string `json:"admin_role"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	identity := identityFromContext(r.Context())
	role := strings.TrimSpace(request.AdminRole)
	if role == "" && request.IsAdmin != nil {
		role = "none"
		if *request.IsAdmin {
			role = "owner"
		}
	}
	allowedRoles := map[string]bool{
		"none": true, "read_only": true, "support": true, "editor": true,
		"reviewer": true, "operator": true, "owner": true,
	}
	if !allowedRoles[role] {
		writeAPIError(w, http.StatusBadRequest, "Неизвестная административная роль")
		return
	}
	if identity.ID == userID && role != "owner" {
		writeAPIError(w, http.StatusBadRequest, "Нельзя снять роль администратора у текущей сессии")
		return
	}
	if err := s.store.UpdateUserAdminRole(r.Context(), userID, role); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "Пользователь не найден")
			return
		}
		if errors.Is(err, ErrConflict) {
			writeAPIError(w, http.StatusConflict, "В системе должен оставаться хотя бы один владелец")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить роль")
		return
	}
	s.writeAudit(r.Context(), identity, "update_admin_role", "user", userID,
		map[string]any{"admin_role": role}, s.requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.AuditLogs(r.Context(), queryInt(r, "limit", 100))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить аудит")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) beginSync(sourceID string) bool {
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	if s.running[sourceID] {
		return false
	}
	s.running[sourceID] = true
	return true
}

func (s *Server) finishSync(sourceID string) {
	s.runningMu.Lock()
	delete(s.running, sourceID)
	s.runningMu.Unlock()
}

func (s *Server) enrichRunning(sources []SourceView) {
	s.runningMu.RLock()
	defer s.runningMu.RUnlock()
	for i := range sources {
		if s.running[sources[i].ID] {
			sources[i].Running = true
			sources[i].Health = "running"
		}
	}
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://telegram.org; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; font-src 'self'; base-uri 'self'; form-action 'self'; object-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

type responseCapture struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *responseCapture) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseCapture) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, requestID)))
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		capture := &responseCapture{ResponseWriter: w}
		next.ServeHTTP(capture, r)
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/health" {
			return
		}
		status := capture.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("admin request",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", capture.bytes,
			"elapsed", time.Since(started),
			"admin_id", r.Header.Get(internalAdminIDHeader),
			"ip", s.requestIP(r),
		)
	})
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("admin request panic",
					"request_id", requestIDFromContext(r.Context()),
					"panic", recovered,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				if capture, ok := w.(*responseCapture); !ok || capture.status == 0 {
					writeAPIError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func spaHandler(assets fs.FS, index []byte) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeAPIError(w, http.StatusNotFound, "API endpoint не найден")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if info, err := fs.Stat(assets, path); err == nil && !info.IsDir() {
				if strings.HasPrefix(path, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"code":       httpErrorCode(status),
		"error":      message,
		"status":     status,
		"request_id": w.Header().Get(requestIDHeader),
	})
}

func httpErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "request.invalid"
	case http.StatusUnauthorized:
		return "auth.required"
	case http.StatusForbidden:
		return "auth.forbidden"
	case http.StatusNotFound:
		return "resource.not_found"
	case http.StatusConflict:
		return "resource.conflict"
	case http.StatusTooManyRequests:
		return "request.rate_limited"
	case http.StatusServiceUnavailable:
		return "service.unavailable"
	default:
		return "service.internal_error"
	}
}

func queryInt(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return value
}

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func parseTrustedProxies(raw string) ([]*net.IPNet, error) {
	var result []*net.IPNet
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("admin trusted proxy %q: %w", value, err)
		}
		result = append(result, network)
	}
	return result, nil
}

func (s *Server) requestIP(r *http.Request) string {
	remote := remoteIP(r.RemoteAddr)
	if remote == nil || !s.isTrustedProxy(remote) {
		if remote != nil {
			return remote.String()
		}
		return r.RemoteAddr
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if candidate, valid := s.forwardedClientIP(remote, forwarded); valid {
			return candidate.String()
		}
		return remote.String()
	}
	if candidate := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); candidate != nil {
		return candidate.String()
	}
	return remote.String()
}

func (s *Server) forwardedClientIP(remote net.IP, value string) (net.IP, bool) {
	current := remote
	parts := strings.Split(value, ",")
	for index := len(parts) - 1; index >= 0; index-- {
		if !s.isTrustedProxy(current) {
			return current, true
		}
		candidate := net.ParseIP(strings.TrimSpace(parts[index]))
		if candidate == nil {
			return nil, false
		}
		current = candidate
	}
	return current, true
}

func (s *Server) isTrustedProxy(ip net.IP) bool {
	for _, network := range s.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(address string) net.IP {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(address)
}
