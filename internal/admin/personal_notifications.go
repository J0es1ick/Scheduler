package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

func (s *Server) handlePersonalNotifications(w http.ResponseWriter, r *http.Request) {
	userID := identityFromContext(r.Context()).ID
	users := repository.NewUserRepository(s.store.db)
	if r.Method == http.MethodPut {
		var input domain.NotificationPreferences
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "Некорректные настройки")
			return
		}
		if err := service.NewUserService(users).SaveNotificationPreferences(r.Context(), userID, input); err != nil {
			if errors.Is(err, repository.ErrPersonalConflict) {
				writeAPIError(w, http.StatusConflict, "Настройки уже изменились. Загрузите сохранённые настройки и повторите")
			} else {
				s.personalError(w, err)
			}
			return
		}
	}
	user, err := users.GetUserByID(r.Context(), userID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	if user == nil {
		s.personalError(w, ErrUnauthorized)
		return
	}
	targets, err := s.personalSchedule.PersonalTargets(r.Context(), userID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	p := user.NotificationPreferences()
	zone := "Europe/Moscow"
	preview := []service.PlannedNotification{}
	for _, target := range targets {
		if !target.Primary {
			continue
		}
		zone = target.Timezone
		location, err := time.LoadLocation(zone)
		if err != nil {
			s.personalError(w, err)
			return
		}
		now := time.Now().In(location)
		agenda, err := s.personalSchedule.PersonalAgenda(r.Context(), userID, target.ID, now, now.AddDate(0, 0, 7))
		if err != nil {
			s.personalError(w, err)
			return
		}
		preview = service.PreviewNotifications(p, agenda, now)
		break
	}
	writeJSON(w, http.StatusOK, map[string]any{"preferences": p, "timezone": zone, "preview": preview})
}

func (s *Server) handlePersonalAgenda(w http.ResponseWriter, r *http.Request) {
	userID := identityFromContext(r.Context()).ID
	targets, err := s.personalSchedule.PersonalTargets(r.Context(), userID)
	if err != nil {
		s.personalError(w, err)
		return
	}
	for _, target := range targets {
		if target.ID != r.URL.Query().Get("target") {
			continue
		}
		location, err := time.LoadLocation(target.Timezone)
		if err != nil {
			s.personalError(w, err)
			return
		}
		now := time.Now().In(location)
		agenda, err := s.personalSchedule.PersonalAgenda(r.Context(), userID, target.ID, now, now.AddDate(0, 0, 7))
		if err != nil {
			s.personalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agenda)
		return
	}
	writeAPIError(w, http.StatusNotFound, "Расписание недоступно")
}
