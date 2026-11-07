package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/service/validation"
)

type PersonalNoteInput struct {
	ID        string `json:"id"`
	Version   int64  `json:"version"`
	TargetID  string `json:"target_id"`
	LessonID  string `json:"lesson_id"`
	Date      string `json:"date"`
	Body      string `json:"body"`
	DueDate   string `json:"due_date"`
	Completed bool   `json:"completed"`
	Rebind    bool   `json:"rebind"`
}

func (s *ScheduleService) SavePersonalNote(ctx context.Context, userID string, input PersonalNoteInput) (*domain.PersonalNote, error) {
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || utf8.RuneCountInString(input.Body) > 4000 || strings.ContainsRune(input.Body, 0) {
		return nil, validation.PersonalInput("Напишите заметку от 1 до 4000 символов")
	}
	item := domain.PersonalNote{ID: input.ID, UserID: userID, Body: input.Body, Completed: input.Completed, Basis: json.RawMessage(`{}`)}
	if input.DueDate != "" {
		date, err := time.Parse(time.DateOnly, input.DueDate)
		if err != nil || len(input.DueDate) != 10 {
			return nil, validation.PersonalInput("Укажите срок в формате ГГГГ-ММ-ДД")
		}
		item.DueDate = &date
	}
	if input.ID == "" || input.Rebind {
		date, err := time.Parse(time.DateOnly, input.Date)
		if err != nil {
			return nil, validation.PersonalInput("Выберите дату занятия")
		}
		agenda, err := s.PersonalAgenda(ctx, userID, input.TargetID, date, date)
		if err != nil {
			return nil, err
		}
		found := false
		for _, lesson := range agenda.Days[0].Lessons {
			if lesson.Original.ID == input.LessonID || lesson.Original.PersonalKey == input.LessonID {
				item.Role = agenda.Target.Role
				item.TargetID = agenda.Target.ID
				item.LessonDate = date
				item.LessonID = lesson.Original.PersonalKey
				if item.LessonID == "" {
					item.LessonID = lesson.Original.ID
				}
				item.Basis, err = json.Marshal(lesson.Original)
				if err != nil {
					return nil, err
				}
				found = true
				break
			}
		}
		if !found {
			return nil, validation.PersonalInput("Занятие изменилось. Обновите расписание и выберите его снова")
		}
	}
	return s.personalRepo.SaveNote(ctx, item, input.Version, input.Rebind)
}

type PersonalNoteView struct {
	domain.PersonalNote
	Binding         string `json:"binding"`
	CurrentDate     string `json:"current_date,omitempty"`
	CurrentLessonID string `json:"current_lesson_id,omitempty"`
}

func ResolveNoteBinding(note domain.PersonalNote, agenda *PersonalAgenda) PersonalNoteView {
	result := PersonalNoteView{PersonalNote: note, Binding: "needs_review"}
	if agenda == nil {
		return result
	}
	var basis domain.Lesson
	if json.Unmarshal(note.Basis, &basis) != nil {
		return result
	}
	type match struct {
		date string
		item PersonalLesson
	}
	exact, stable := []match{}, []match{}
	for _, day := range agenda.Days {
		for _, item := range day.Lessons {
			l := item.Original
			if l.GroupID != basis.GroupID || l.Subgroup != basis.Subgroup || searchtext.TokenKey(l.Subject) != searchtext.TokenKey(basis.Subject) || l.Type != basis.Type {
				continue
			}
			m := match{day.Date, item}
			identity := l.ID == note.LessonID || l.PersonalKey == note.LessonID
			if identity {
				stable = append(stable, m)
			}
			if day.Date == note.LessonDate.Format(time.DateOnly) {
				if identity || (l.TimeStart == basis.TimeStart && l.TimeEnd == basis.TimeEnd) {
					exact = append(exact, m)
				}
			}
		}
	}
	chosen := stable
	if len(exact) > 0 {
		chosen = exact
	}
	if len(chosen) != 1 {
		return result
	}
	result.CurrentDate = chosen[0].date
	result.CurrentLessonID = chosen[0].item.Original.ID
	result.Binding = "attached"
	if result.CurrentDate != note.LessonDate.Format(time.DateOnly) || chosen[0].item.Original.TimeStart != basis.TimeStart {
		result.Binding = "moved"
	}
	if chosen[0].item.Cancelled {
		result.Binding = "cancelled"
	}
	return result
}

func (s *ScheduleService) PersonalNotes(ctx context.Context, userID string) ([]PersonalNoteView, error) {
	notes, err := s.personalRepo.Notes(ctx, userID)
	if err != nil {
		return nil, err
	}
	targets, err := s.PersonalTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, target := range targets {
		allowed[string(target.Role)+":"+target.ID] = true
	}
	result := []PersonalNoteView{}
	cache := map[string]*PersonalAgenda{}
	for _, note := range notes {
		var agenda *PersonalAgenda
		if allowed[string(note.Role)+":"+note.TargetID] {
			from := note.LessonDate.AddDate(0, 0, -((int(note.LessonDate.Weekday()) + 6) % 7))
			key := note.TargetID + ":" + from.Format(time.DateOnly)
			var exists bool
			agenda, exists = cache[key]
			if !exists {
				agenda, err = s.PersonalAgenda(ctx, userID, note.TargetID, from, from.AddDate(0, 0, 6))
				if err != nil {
					return nil, err
				}
				cache[key] = agenda
			}
		}
		result = append(result, ResolveNoteBinding(note, agenda))
	}
	return result, nil
}
