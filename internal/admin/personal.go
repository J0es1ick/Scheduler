package admin

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

type personalSessionStore struct{ store *Store }

func (s *personalSessionStore) SaveAdminSession(ctx context.Context, hash string, identity AdminIdentity, expires time.Time, _ int) error {
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner string
	if err = tx.GetContext(ctx, &owner, `SELECT id FROM users WHERE id=$1 AND NOT bot_blocked FOR UPDATE`, identity.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM personal_sessions WHERE expires_at<=NOW()`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO personal_sessions(token_hash,user_id,name,csrf_token,expires_at) VALUES($1,$2,$3,$4,$5)`, hash, identity.ID, identity.Name, identity.CSRFToken, expires); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM personal_sessions WHERE token_hash IN(SELECT token_hash FROM personal_sessions WHERE user_id=$1 ORDER BY created_at DESC,token_hash OFFSET 10)`, identity.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *personalSessionStore) AdminSession(ctx context.Context, hash string) (AdminIdentity, time.Time, error) {
	var row struct {
		AdminIdentity
		Expires time.Time `db:"expires_at"`
	}
	err := s.store.db.GetContext(ctx, &row, `SELECT user_id AS id,name,'telegram' AS auth_method,'none' AS role,csrf_token,expires_at FROM personal_sessions WHERE token_hash=$1 AND expires_at>NOW()`, hash)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrUnauthorized
	}
	return row.AdminIdentity, row.Expires, err
}
func (s *personalSessionStore) DeleteAdminSession(ctx context.Context, hash string) error {
	_, err := s.store.db.ExecContext(ctx, `DELETE FROM personal_sessions WHERE token_hash=$1`, hash)
	return err
}

func (s *Server) registerPersonalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/personal/auth/telegram", s.handlePersonalLogin)
	s.personalRoute(mux, "GET /api/personal/me", s.handlePersonalMe)
	s.personalRoute(mux, "POST /api/personal/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := s.personalAuth.Logout(w, r); err != nil {
			s.personalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	s.personalRoute(mux, "GET /api/personal/schedule", s.handlePersonalSchedule)
	s.personalRoute(mux, "POST /api/personal/changes", s.handlePersonalSave)
	s.personalRoute(mux, "DELETE /api/personal/changes/{id}", s.handlePersonalDelete)
}

func (s *Server) personalRoute(mux *http.ServeMux, pattern string, next http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		identity, err := s.personalAuth.identityForRequest(r)
		if err != nil {
			s.personalError(w, err)
			return
		}
		user, err := repository.NewUserRepository(s.store.db).GetUserByID(r.Context(), identity.ID)
		if err != nil {
			s.personalError(w, err)
			return
		}
		if user == nil || user.BotBlocked {
			s.personalError(w, ErrForbidden)
			return
		}
		if r.Method != http.MethodGet && !constantTimeTokenEqual(r.Header.Get("X-CSRF-Token"), identity.CSRFToken) {
			s.personalError(w, ErrForbidden)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity)))
	})
}

func (s *Server) handlePersonalLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	key := "personal:" + s.requestIP(r)
	if allowed, retry := s.logins.allowed(key, time.Now()); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()))))
		writeAPIError(w, http.StatusTooManyRequests, "Слишком много попыток входа")
		return
	}
	var input struct {
		InitData string `json:"init_data"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	telegramUser, err := validateTelegramInitData(input.InitData, s.auth.botToken, time.Now())
	if err != nil {
		s.logins.failed(key, time.Now())
		s.personalError(w, ErrUnauthorized)
		return
	}
	userID := strconv.FormatInt(telegramUser.ID, 10)
	user, err := repository.NewUserRepository(s.store.db).GetUserByID(r.Context(), userID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	if user == nil {
		writeAPIError(w, http.StatusForbidden, "Сначала откройте бота и завершите настройку через /start")
		return
	}
	if user.BotBlocked {
		s.personalError(w, ErrForbidden)
		return
	}
	identity, err := s.personalAuth.IssueSession(w, AdminIdentity{ID: user.ID, Name: telegramUser.FirstName, AuthMethod: "telegram", Role: "none"})
	if err != nil {
		s.personalError(w, err)
		return
	}
	s.logins.succeeded(key)
	s.handlePersonalMe(w, r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity)))
}

func (s *Server) handlePersonalMe(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	user, err := repository.NewUserRepository(s.store.db).GetUserByID(r.Context(), identity.ID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	if user == nil {
		s.personalError(w, ErrUnauthorized)
		return
	}
	targets, err := s.personalSchedule.PersonalTargets(r.Context(), identity.ID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "name": identity.Name, "is_admin": user.IsAdmin, "role": user.Role, "csrf_token": identity.CSRFToken, "targets": targets})
}

func (s *Server) handlePersonalSchedule(w http.ResponseWriter, r *http.Request) {
	from, e1 := time.Parse(time.DateOnly, r.URL.Query().Get("from"))
	to, e2 := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 30*24*time.Hour {
		writeAPIError(w, http.StatusBadRequest, "Выберите период до 31 дня")
		return
	}
	result, err := s.personalSchedule.PersonalSchedule(r.Context(), identityFromContext(r.Context()).ID, r.URL.Query().Get("target"), from, to)
	if err != nil {
		s.personalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePersonalSave(w http.ResponseWriter, r *http.Request) {
	var input service.PersonalChangeInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректная правка")
		return
	}
	result, err := s.personalSchedule.SavePersonalChange(r.Context(), identityFromContext(r.Context()).ID, input)
	if err != nil {
		s.personalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePersonalDelete(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err != nil || version < 1 {
		writeAPIError(w, http.StatusBadRequest, "Обновите список правок")
		return
	}
	err = repository.NewPersonalScheduleRepository(s.store.db).Delete(r.Context(), identityFromContext(r.Context()).ID, r.PathValue("id"), version)
	if err != nil {
		s.personalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) personalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeAPIError(w, http.StatusUnauthorized, "Откройте приложение заново из Telegram")
	case errors.Is(err, ErrForbidden), errors.Is(err, sql.ErrNoRows):
		writeAPIError(w, http.StatusForbidden, "Это расписание недоступно. Проверьте профиль в боте")
	case errors.Is(err, repository.ErrPersonalConflict):
		writeAPIError(w, http.StatusConflict, "Правка уже изменилась. Обновите расписание и повторите")
	case errors.Is(err, service.ErrPersonalInput):
		writeAPIError(w, http.StatusBadRequest, err.Error()[len(service.ErrPersonalInput.Error())+2:])
	default:
		slog.Error("personal schedule request failed", "err", err)
		writeAPIError(w, http.StatusInternalServerError, "Не удалось сохранить или загрузить личное расписание")
	}
}
