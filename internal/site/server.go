package site

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/buildinfo"
	"github.com/J0es1ick/Scheduler/internal/siteui"
)

type Server struct {
	store      *Store
	publicInfo *publicInfoCache
	projectURL string
	botURL     string
	handler    http.Handler
}

func NewServer(store *Store, projectURL, botURL string) (*Server, error) {
	server := &Server{
		store:      store,
		publicInfo: newPublicInfoCache(),
		projectURL: projectURL,
		botURL:     botURL,
	}
	assets, err := siteui.Files()
	if err != nil {
		return nil, fmt.Errorf("site UI assets: %w", err)
	}
	index, err := siteui.Index()
	if err != nil {
		return nil, fmt.Errorf("site UI index: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.handleHealth)
	mux.HandleFunc("GET /api/ready", server.handleReady)
	mux.HandleFunc("GET /api/public-info", server.handlePublicInfo)
	mux.Handle("/", spaHandler(assets, index))
	server.handler = securityHeaders(requestLog(mux))
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
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
		writeError(w, http.StatusServiceUnavailable, "База данных недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handlePublicInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	info, err := s.publicInfo.Get(ctx, func(loadContext context.Context) (*PublicInfo, error) {
		return s.store.PublicInfo(loadContext, s.projectURL, s.botURL)
	})
	if err != nil {
		slog.Error("public project info failed", "err", err)
		writeError(w, http.StatusServiceUnavailable, "Статистика проекта временно недоступна")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, info)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self'; font-src 'self'; "+
				"base-uri 'self'; form-action 'self'; object-src 'none'; frame-ancestors 'none'",
		)
		next.ServeHTTP(w, r)
	})
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			slog.Debug(
				"site request",
				"method", r.Method,
				"path", r.URL.Path,
				"elapsed", time.Since(started),
			)
		}
	})
}

func spaHandler(assets fs.FS, index []byte) http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "API endpoint не найден")
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":  message,
		"status": status,
	})
}
