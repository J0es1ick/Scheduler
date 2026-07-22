package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/adminui"
	"github.com/J0es1ick/Scheduler/internal/service"
)

type Server struct {
	store  *Store
	auth   *AuthManager
	parser *service.ParserService

	runningMu sync.RWMutex
	running   map[string]bool
	handler   http.Handler
}

func NewServer(store *Store, auth *AuthManager, parser *service.ParserService) (*Server, error) {
	server := &Server{
		store:   store,
		auth:    auth,
		parser:  parser,
		running: make(map[string]bool),
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
	mux.HandleFunc("POST /api/auth/access-key", server.handleAccessKeyLogin)
	mux.HandleFunc("POST /api/auth/telegram", server.handleTelegramLogin)
	server.protected(mux, "GET /api/auth/me", server.handleMe)
	server.protected(mux, "POST /api/auth/logout", server.handleLogout)
	server.protected(mux, "GET /api/dashboard", server.handleDashboard)
	server.protected(mux, "GET /api/sources", server.handleSources)
	server.protected(mux, "PATCH /api/sources/{id}", server.handleUpdateSource)
	server.protected(mux, "POST /api/sources/{id}/sync", server.handleSyncSource)
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

	server.handler = server.recoverPanic(server.securityHeaders(server.requestLog(mux)))
	return server, nil
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) protected(mux *http.ServeMux, pattern string, handler http.HandlerFunc) {
	mux.Handle(pattern, s.auth.Require(s.store, handler))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now()})
}

func (s *Server) handleAccessKeyLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		AccessKey string `json:"access_key"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	identity, err := s.auth.LoginWithAccessKey(request.AccessKey)
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, "Неверный ключ администратора")
		return
	}
	identity, err = s.auth.IssueSession(w, identity)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось создать сессию")
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "login", "session", "", map[string]any{"method": "access_key"}, requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"user": identity})
}

func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		InitData string `json:"init_data"`
	}
	if err := decodeJSON(r, &request); err != nil {
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
	_ = s.store.WriteAudit(r.Context(), identity, "login", "session", "", map[string]any{"method": "telegram"}, requestIP(r))
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
	_ = s.store.WriteAudit(r.Context(), identity, "logout", "session", "", map[string]any{}, requestIP(r))
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
	sources, err := s.store.Sources(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Не удалось загрузить источники")
		return
	}
	s.enrichRunning(sources)
	writeJSON(w, http.StatusOK, map[string]any{"items": sources})
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	var request struct {
		UpdateInterval int `json:"update_interval"`
	}
	if err := decodeJSON(r, &request); err != nil || request.UpdateInterval < 300 || request.UpdateInterval > 604800 {
		writeAPIError(w, http.StatusBadRequest, "Интервал должен быть от 5 минут до 7 дней")
		return
	}
	if err := s.store.UpdateSourceInterval(r.Context(), sourceID, request.UpdateInterval); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "Источник не найден")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить интервал")
		return
	}
	identity := identityFromContext(r.Context())
	_ = s.store.WriteAudit(r.Context(), identity, "update_interval", "data_source", sourceID,
		map[string]any{"update_interval": request.UpdateInterval}, requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (s *Server) handleSyncSource(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")
	if sourceID == "" {
		writeAPIError(w, http.StatusBadRequest, "Источник не указан")
		return
	}
	if !s.beginSync(sourceID) {
		writeAPIError(w, http.StatusConflict, "Этот источник уже обновляется")
		return
	}
	identity := identityFromContext(r.Context())
	ipAddress := requestIP(r)
	_ = s.store.WriteAudit(r.Context(), identity, "sync_requested", "data_source", sourceID, map[string]any{}, ipAddress)
	go func() {
		defer s.finishSync(sourceID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		records, err := s.parser.RunDataSource(ctx, sourceID)
		details := map[string]any{"records": records}
		action := "sync_completed"
		if err != nil {
			action = "sync_failed"
			details["error"] = err.Error()
			slog.Error("admin manual sync failed", "source", sourceID, "err", err)
		} else {
			slog.Info("admin manual sync complete", "source", sourceID, "records", records)
		}
		_ = s.store.WriteAudit(context.Background(), identity, action, "data_source", sourceID, details, ipAddress)
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
	if err := decodeJSON(r, &request); err != nil {
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
	_ = s.store.WriteAudit(r.Context(), identity, "resolve_support_request", "support_request", requestID,
		map[string]any{"status": request.Status, "review_note": request.ReviewNote}, requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": request.Status})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	var request struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	identity := identityFromContext(r.Context())
	if identity.ID == userID && !request.IsAdmin {
		writeAPIError(w, http.StatusBadRequest, "Нельзя снять роль администратора у текущей сессии")
		return
	}
	if err := s.store.UpdateUserAdmin(r.Context(), userID, request.IsAdmin); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeAPIError(w, http.StatusNotFound, "Пользователь не найден")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить роль")
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "update_admin_role", "user", userID,
		map[string]any{"is_admin": request.IsAdmin}, requestIP(r))
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://telegram.org; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self'; font-src 'self'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			slog.Debug("admin request", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(started))
		}
	})
}

func (s *Server) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("admin request panic", "panic", recovered, "path", r.URL.Path)
				writeAPIError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
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

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "status": status})
}

func queryInt(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return value
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
