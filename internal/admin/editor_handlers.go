package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type lessonMutationRequest struct {
	GroupID           string `json:"group_id"`
	SemesterID        string `json:"semester_id"`
	DayOfWeek         int    `json:"day_of_week"`
	SpecialDate       string `json:"special_date"`
	TimeStart         string `json:"time_start"`
	TimeEnd           string `json:"time_end"`
	WeekType          string `json:"week_type"`
	Subject           string `json:"subject"`
	Type              string `json:"type"`
	Teacher           string `json:"teacher"`
	Room              string `json:"room"`
	Subgroup          int    `json:"subgroup"`
	ValidFrom         string `json:"valid_from"`
	ValidTo           string `json:"valid_to"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

func (s *Server) handleEditorSchedule(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group"))
	if groupID == "" {
		writeAPIError(w, http.StatusBadRequest, "Выберите учебную группу")
		return
	}
	schedule, err := s.store.EditorSchedule(r.Context(), groupID)
	if err != nil {
		writeEditorError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (s *Server) handleCreateEditorLesson(w http.ResponseWriter, r *http.Request) {
	var request lessonMutationRequest
	if err := decodeJSON(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	lesson, err := request.lesson(true)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	identity := identityFromContext(r.Context())
	id, err := s.store.CreateManualLesson(r.Context(), identity.ID, lesson)
	if err != nil {
		writeEditorError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "create_lesson", "lesson", id,
		map[string]any{"group_id": lesson.GroupID, "subject": lesson.Subject}, requestIP(r))
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleUpdateEditorLesson(w http.ResponseWriter, r *http.Request) {
	var request lessonMutationRequest
	if err := decodeJSON(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	lesson, err := request.lesson(false)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	expected, err := parseExpectedTime(request.ExpectedUpdatedAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "Запись была загружена некорректно; обновите страницу")
		return
	}
	identity := identityFromContext(r.Context())
	resultID, err := s.store.UpdateEditorLesson(
		r.Context(), identity.ID, r.PathValue("id"), expected, lesson,
	)
	if err != nil {
		writeEditorError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "update_lesson", "lesson", resultID,
		map[string]any{"subject": lesson.Subject}, requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"id": resultID})
}

func (s *Server) handleDeleteEditorLesson(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedUpdatedAt string `json:"expected_updated_at"`
		Subject           string `json:"subject"`
		GroupID           string `json:"group_id"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "Некорректный запрос")
		return
	}
	expected, err := parseExpectedTime(request.ExpectedUpdatedAt)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "Запись была загружена некорректно; обновите страницу")
		return
	}
	identity := identityFromContext(r.Context())
	lessonID := r.PathValue("id")
	if err = s.store.DeleteEditorLesson(r.Context(), identity.ID, lessonID, expected); err != nil {
		writeEditorError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "delete_lesson", "lesson", lessonID,
		map[string]any{"group_id": request.GroupID, "subject": request.Subject}, requestIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreEditorLesson(w http.ResponseWriter, r *http.Request) {
	identity := identityFromContext(r.Context())
	lessonID := r.PathValue("id")
	if err := s.store.RestoreEditorLesson(r.Context(), lessonID); err != nil {
		writeEditorError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), identity, "restore_lesson", "lesson", lessonID,
		map[string]any{}, requestIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"status": "restored"})
}

func (r lessonMutationRequest) lesson(requireGroup bool) (LessonMutation, error) {
	r.GroupID = strings.TrimSpace(r.GroupID)
	r.SemesterID = strings.TrimSpace(r.SemesterID)
	r.Subject = strings.TrimSpace(r.Subject)
	r.Teacher = strings.TrimSpace(r.Teacher)
	r.Room = strings.TrimSpace(r.Room)
	r.TimeStart = strings.TrimSpace(r.TimeStart)
	r.TimeEnd = strings.TrimSpace(r.TimeEnd)
	r.WeekType = strings.TrimSpace(r.WeekType)
	r.Type = strings.TrimSpace(r.Type)

	if requireGroup && r.GroupID == "" {
		return LessonMutation{}, errors.New("выберите учебную группу")
	}
	if r.SemesterID == "" {
		return LessonMutation{}, errors.New("выберите семестр")
	}
	if r.Subject == "" || utf8.RuneCountInString(r.Subject) > 300 {
		return LessonMutation{}, errors.New("укажите название предмета длиной до 300 символов")
	}
	if utf8.RuneCountInString(r.Teacher) > 200 || utf8.RuneCountInString(r.Room) > 100 {
		return LessonMutation{}, errors.New("преподаватель или аудитория указаны слишком длинно")
	}
	if r.Subgroup < 0 || r.Subgroup > 10 {
		return LessonMutation{}, errors.New("номер подгруппы должен быть от 0 до 10")
	}
	if !oneOf(r.WeekType, "every", "odd", "even", "date") {
		return LessonMutation{}, errors.New("выберите режим повторения занятия")
	}
	if !oneOf(r.Type, "lecture", "practice", "lab", "seminar", "exam", "credit", "consultation") {
		return LessonMutation{}, errors.New("выберите тип занятия")
	}
	start, startErr := time.Parse("15:04", r.TimeStart)
	end, endErr := time.Parse("15:04", r.TimeEnd)
	if startErr != nil || endErr != nil || !end.After(start) {
		return LessonMutation{}, errors.New("проверьте время начала и окончания")
	}

	specialDate, err := parseOptionalDate(r.SpecialDate)
	if err != nil {
		return LessonMutation{}, errors.New("проверьте дату занятия")
	}
	validFrom, err := parseOptionalDate(r.ValidFrom)
	if err != nil {
		return LessonMutation{}, errors.New("проверьте дату начала действия")
	}
	validTo, err := parseOptionalDate(r.ValidTo)
	if err != nil {
		return LessonMutation{}, errors.New("проверьте дату окончания действия")
	}
	if (validFrom == nil) != (validTo == nil) {
		return LessonMutation{}, errors.New("период действия должен содержать обе даты")
	}
	if validFrom != nil && validTo.Before(*validFrom) {
		return LessonMutation{}, errors.New("окончание периода не может быть раньше начала")
	}
	if r.WeekType == "date" {
		if specialDate == nil {
			return LessonMutation{}, errors.New("укажите точную дату занятия")
		}
		r.DayOfWeek = 0
		validFrom, validTo = specialDate, specialDate
	} else {
		if r.DayOfWeek < 1 || r.DayOfWeek > 7 {
			return LessonMutation{}, errors.New("выберите день недели")
		}
		specialDate = nil
	}

	return LessonMutation{
		GroupID: r.GroupID, SemesterID: r.SemesterID, DayOfWeek: r.DayOfWeek,
		SpecialDate: specialDate, TimeStart: r.TimeStart, TimeEnd: r.TimeEnd,
		WeekType: r.WeekType, Subject: r.Subject, Type: r.Type,
		Teacher: r.Teacher, Room: r.Room, Subgroup: r.Subgroup,
		ValidFrom: validFrom, ValidTo: validTo,
	}, nil
}

func parseOptionalDate(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseExpectedTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func writeEditorError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "Группа, семестр или занятие не найдены")
	case errors.Is(err, ErrConflict):
		writeAPIError(w, http.StatusConflict, "Расписание уже изменилось. Обновите данные и повторите действие")
	default:
		writeAPIError(w, http.StatusInternalServerError, "Не удалось изменить расписание")
	}
}
