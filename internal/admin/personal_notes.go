package admin

import (
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"net/http"
	"strconv"
)

func (s *Server) handlePersonalNotes(w http.ResponseWriter, r *http.Request) {
	userID := identityFromContext(r.Context()).ID
	if r.Method == http.MethodGet {
		items, err := s.personalSchedule.PersonalNotes(r.Context(), userID)
		if err != nil {
			s.personalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
		return
	}
	if r.Method == http.MethodDelete {
		version, err := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
		if err != nil || version < 1 {
			writeAPIError(w, http.StatusBadRequest, "Обновите заметку перед удалением")
			return
		}
		if err = repository.NewPersonalScheduleRepository(s.store.db).DeleteNote(r.Context(), userID, r.PathValue("id"), version); err != nil {
			s.personalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var input service.PersonalNoteInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректная заметка")
		return
	}
	item, err := s.personalSchedule.SavePersonalNote(r.Context(), userID, input)
	if err != nil {
		s.personalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
