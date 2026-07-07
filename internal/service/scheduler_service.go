package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

type ScheduleService struct {
	lessonRepo   *repository.LessonRepository
	semesterRepo *repository.SemesterRepository
	groupRepo    *repository.GroupRepository
}

func NewScheduleService(
	lessonRepo *repository.LessonRepository,
	semesterRepo *repository.SemesterRepository,
	groupRepo *repository.GroupRepository,
) *ScheduleService {
	return &ScheduleService{
		lessonRepo:   lessonRepo,
		semesterRepo: semesterRepo,
		groupRepo:    groupRepo,
	}
}

func (s *ScheduleService) GetScheduleForGroup(ctx context.Context, groupID string, date time.Time) ([]domain.Lesson, error) {
	date = helpers.NormalizeDate(date)

	group, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("group not found")
	}

	semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
	if err != nil {
		return nil, err
	}
	if semester == nil {
		return nil, fmt.Errorf("no semester found for date %v", date)
	}

	allLessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	var result []domain.Lesson
	weekType := helpers.DetermineWeekType(date, semester.StartDate)
	dayOfWeek := helpers.Weekday(date)

	for _, lesson := range allLessons {
		if lesson.SemesterID != semester.ID {
			continue
		}
		if lesson.SpecialDate != nil && helpers.NormalizeDate(*lesson.SpecialDate).Equal(date) {
			result = append(result, lesson)
			continue
		}
		if lesson.SpecialDate == nil {
			if lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
				result = append(result, lesson)
			}
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForGroupRange(ctx context.Context, groupID string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	from = helpers.NormalizeDate(from)
	to = helpers.NormalizeDate(to)

	group, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil || group == nil {
		return nil, fmt.Errorf("group not found")
	}

	semesters, err := s.semesterRepo.GetSemestersByUniversityID(ctx, group.UniversityID)
	if err != nil {
		return nil, err
	}

	allLessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result[date] = []domain.Lesson{}
	}

	for _, lesson := range allLessons {
		var semester *domain.Semester
		for _, s := range semesters {
			if s.ID == lesson.SemesterID {
				semester = &s
				break
			}
		}
		if semester == nil {
			continue
		}

		if lesson.SpecialDate != nil {
			normalized := helpers.NormalizeDate(*lesson.SpecialDate)
			if _, ok := result[normalized]; ok {
				result[normalized] = append(result[normalized], lesson)
			}
			continue
		}

		for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
			dayOfWeek := helpers.Weekday(date)
			weekType := helpers.DetermineWeekType(date, semester.StartDate)
			if lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
				result[date] = append(result[date], lesson)
			}
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForTeacher(ctx context.Context, teacherName string, date time.Time) ([]domain.Lesson, error) {
	date = helpers.NormalizeDate(date)

	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}

	var result []domain.Lesson
	for _, lesson := range lessons {
		group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
		if err != nil || group == nil {
			continue
		}
		semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
		if err != nil || semester == nil {
			continue
		}
		weekType := helpers.DetermineWeekType(date, semester.StartDate)
		dayOfWeek := helpers.Weekday(date)
		if lesson.SpecialDate != nil && helpers.NormalizeDate(*lesson.SpecialDate).Equal(date) {
			result = append(result, lesson)
			continue
		}
		if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
			result = append(result, lesson)
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForTeacherRange(ctx context.Context, teacherName string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	from = helpers.NormalizeDate(from)
	to = helpers.NormalizeDate(to)

	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}

	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result[date] = []domain.Lesson{}
	}

	if len(lessons) == 0 {
		return result, nil
	}

	// Загружаем семестры одним батчем по уникальным group_id → university_id,
	// чтобы избежать N+1 запросов (по одному на каждую пару занятие×дата).
	semCache, err := s.buildSemesterCacheForLessons(ctx, lessons)
	if err != nil {
		return nil, err
	}

	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		dayOfWeek := helpers.Weekday(date)
		for _, lesson := range lessons {
			sem, ok := semCache[lesson.SemesterID]
			if !ok {
				continue
			}
			weekType := helpers.DetermineWeekType(date, sem.StartDate)
			if lesson.SpecialDate != nil && helpers.NormalizeDate(*lesson.SpecialDate).Equal(date) {
				result[date] = append(result[date], lesson)
				continue
			}
			if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
				result[date] = append(result[date], lesson)
			}
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForRoom(ctx context.Context, room string, date time.Time) ([]domain.Lesson, error) {
	date = helpers.NormalizeDate(date)

	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}

	var result []domain.Lesson
	for _, lesson := range lessons {
		group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
		if err != nil || group == nil {
			continue
		}
		semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
		if err != nil || semester == nil {
			continue
		}
		weekType := helpers.DetermineWeekType(date, semester.StartDate)
		dayOfWeek := helpers.Weekday(date)
		if lesson.SpecialDate != nil && helpers.NormalizeDate(*lesson.SpecialDate).Equal(date) {
			result = append(result, lesson)
			continue
		}
		if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
			result = append(result, lesson)
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForRoomRange(ctx context.Context, room string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	from = helpers.NormalizeDate(from)
	to = helpers.NormalizeDate(to)

	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}

	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result[date] = []domain.Lesson{}
	}

	if len(lessons) == 0 {
		return result, nil
	}

	semCache, err := s.buildSemesterCacheForLessons(ctx, lessons)
	if err != nil {
		return nil, err
	}

	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		dayOfWeek := helpers.Weekday(date)
		for _, lesson := range lessons {
			sem, ok := semCache[lesson.SemesterID]
			if !ok {
				continue
			}
			weekType := helpers.DetermineWeekType(date, sem.StartDate)
			if lesson.SpecialDate != nil && helpers.NormalizeDate(*lesson.SpecialDate).Equal(date) {
				result[date] = append(result[date], lesson)
				continue
			}
			if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && helpers.MatchesWeekType(lesson.WeekType, weekType) {
				result[date] = append(result[date], lesson)
			}
		}
	}
	return result, nil
}

// buildSemesterCacheForLessons загружает все уникальные семестры из среза занятий
// одним батч-запросом и возвращает map[semesterID]*Semester.
// Устраняет N+1: вместо запроса на каждое занятие — один IN-запрос.
func (s *ScheduleService) buildSemesterCacheForLessons(ctx context.Context, lessons []domain.Lesson) (map[string]*domain.Semester, error) {
	semIDs := make(map[string]bool, len(lessons))
	for _, l := range lessons {
		semIDs[l.SemesterID] = true
	}
	ids := make([]string, 0, len(semIDs))
	for id := range semIDs {
		ids = append(ids, id)
	}

	sems, err := s.semesterRepo.GetSemestersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	cache := make(map[string]*domain.Semester, len(sems))
	for i := range sems {
		cache[sems[i].ID] = &sems[i]
	}
	return cache, nil
}

// SplitMessage разбивает длинный текст на части не длиннее maxLen символов,
// разрезая по блокам (двойной перевод строки между днями расписания).
// Нужен для обхода лимита Telegram в 4096 символов на одно сообщение.
func SplitMessage(text string, maxLen int) []string {
	if len([]rune(text)) <= maxLen {
		return []string{text}
	}

	var parts []string
	blocks := strings.Split(text, "\n\n")
	var current strings.Builder

	for _, block := range blocks {
		// +2 для разделителя "\n\n"
		if current.Len() > 0 && len([]rune(current.String()))+len([]rune(block))+2 > maxLen {
			parts = append(parts, strings.TrimRight(current.String(), "\n"))
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(block)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}
// Если занятие с таким ID уже существует — обновляет его поля.
// Все занятия одной группы+семестра сначала удаляются, затем вставляются заново,
// чтобы гарантировать отсутствие «призрачных» занятий, которых больше нет на сайте.
func (s *ScheduleService) SaveLessonsBatch(ctx context.Context, lessons []domain.Lesson) error {
	if len(lessons) == 0 {
		return nil
	}

	// Собираем уникальные пары (semesterID, groupID) для предварительной очистки.
	type key struct{ semesterID, groupID string }
	pairs := make(map[key]bool, len(lessons))
	for _, l := range lessons {
		if l.SemesterID != "" && l.GroupID != "" {
			pairs[key{l.SemesterID, l.GroupID}] = true
		}
	}

	// Удаляем старые занятия для каждой пары, чтобы не оставалось «мёртвых» записей.
	for k := range pairs {
		if err := s.lessonRepo.DeleteLessonsByGroupAndSemester(ctx, k.groupID, k.semesterID); err != nil {
			return fmt.Errorf("SaveLessonsBatch: clear old lessons: %w", err)
		}
	}

	// Вставляем новые занятия через upsert.
	for _, lesson := range lessons {
		if err := s.lessonRepo.UpsertLesson(ctx, lesson); err != nil {
			return fmt.Errorf("SaveLessonsBatch: upsert lesson id=%s: %w", lesson.ID, err)
		}
	}
	return nil
}
