package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/jmoiron/sqlx"
)

var ErrInvalidLesson = errors.New("некорректное правило повторения")

func resolveEditorRule(ctx context.Context, db sqlx.QueryerContext, lesson *LessonMutation, current *EditorLesson) error {
	var start time.Time
	if err := sqlx.GetContext(ctx, db, &start, `SELECT start_date FROM semesters WHERE id=$1`, lesson.SemesterID); err != nil {
		return err
	}
	return resolveRecurrence(lesson, current, start)
}

func resolveRecurrence(lesson *LessonMutation, current *EditorLesson, start time.Time) error {
	if lesson.Recurrence == nil {
		if current != nil {
			if lesson.WeekType != current.WeekType || lesson.SemesterID != current.SemesterID {
				return fmt.Errorf("%w: при смене режима или семестра укажите правило явно", ErrInvalidLesson)
			}
			rule := current.Recurrence
			lesson.Recurrence = &rule
			return nil
		}
		lesson.Recurrence = &domain.RecurrenceRule{}
	}
	if current != nil && lesson.WeekType == current.WeekType && lesson.SemesterID == current.SemesterID && reflect.DeepEqual(*lesson.Recurrence, current.Recurrence) {
		return nil
	}
	rule := lesson.Recurrence
	if lesson.WeekType == "date" && !rule.IsZero() {
		return ErrInvalidLesson
	}
	if rule.IsZero() && (lesson.WeekType == "odd" || lesson.WeekType == "even") {
		week := 1
		if lesson.WeekType == "even" {
			week = 2
		}
		*rule = domain.RecurrenceRule{CycleLength: 2, CycleWeeks: []int{week}, AnchorDate: &start}
	}
	if rule.IsZero() {
		return nil
	}
	if rule.CycleLength < 2 || rule.CycleLength > 16 || len(rule.CycleWeeks) == 0 {
		return ErrInvalidLesson
	}
	if rule.AnchorDate == nil {
		rule.AnchorDate = &start
	}
	seen := map[int]bool{}
	for _, week := range rule.CycleWeeks {
		if week < 1 || week > rule.CycleLength || seen[week] {
			return ErrInvalidLesson
		}
		seen[week] = true
	}
	if lesson.WeekType == "odd" || lesson.WeekType == "even" {
		week := 1
		if lesson.WeekType == "even" {
			week = 2
		}
		if rule.CycleLength != 2 || len(rule.CycleWeeks) != 1 || rule.CycleWeeks[0] != week {
			return ErrInvalidLesson
		}
	}
	return nil
}

func (s *Server) handleEditorPreview(w http.ResponseWriter, r *http.Request) {
	var request struct {
		LessonID string                `json:"lesson_id"`
		Lesson   lessonMutationRequest `json:"lesson"`
		From     string                `json:"from"`
		Days     int                   `json:"days"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, 400, "Некорректный запрос")
		return
	}
	from, err := time.Parse(time.DateOnly, request.From)
	if err != nil || request.Days < 1 || request.Days > 112 {
		writeAPIError(w, 400, "Выберите диапазон от 1 до 112 дней")
		return
	}
	lesson, err := request.Lesson.lesson(false)
	if err != nil {
		writeAPIError(w, 400, err.Error())
		return
	}
	var current *EditorLesson
	if request.LessonID != "" {
		current = &EditorLesson{}
		if err = s.store.db.GetContext(r.Context(), current, `SELECT `+editorLessonColumns+`, FALSE AS is_deleted FROM effective_lessons WHERE id=$1`, request.LessonID); err != nil {
			writeEditorError(w, err)
			return
		}
	}
	if err = resolveEditorRule(r.Context(), s.store.db, &lesson, current); err != nil {
		writeEditorError(w, err)
		return
	}
	value := domain.Lesson{DayOfWeek: lesson.DayOfWeek, SpecialDate: lesson.SpecialDate,
		WeekType: domain.WeekType(lesson.WeekType), ValidFrom: lesson.ValidFrom, ValidTo: lesson.ValidTo, Recurrence: *lesson.Recurrence}
	var start time.Time
	if err = s.store.db.GetContext(r.Context(), &start, `SELECT start_date FROM semesters WHERE id=$1`, lesson.SemesterID); err != nil {
		writeEditorError(w, err)
		return
	}
	dates := []string{}
	for index := 0; index < request.Days; index++ {
		date := from.AddDate(0, 0, index)
		if service.LessonMatchesDate(value, date, &start) {
			dates = append(dates, date.Format(time.DateOnly))
		}
	}
	writeJSON(w, 200, map[string]any{"dates": dates, "recurrence": lesson.Recurrence})
}
