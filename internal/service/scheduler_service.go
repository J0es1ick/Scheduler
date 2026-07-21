package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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
	return &ScheduleService{lessonRepo: lessonRepo, semesterRepo: semesterRepo, groupRepo: groupRepo}
}

func (s *ScheduleService) GetScheduleForGroup(ctx context.Context, groupID string, date time.Time) ([]domain.Lesson, error) {
	group, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || !group.IsActive {
		return nil, fmt.Errorf("group not found")
	}
	lessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return s.lessonsForDate(ctx, lessons, date)
}

func (s *ScheduleService) GetScheduleForGroupRange(ctx context.Context, groupID string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	group, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil || !group.IsActive {
		return nil, fmt.Errorf("group not found")
	}
	lessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	return s.lessonsForRange(ctx, lessons, from, to)
}

func (s *ScheduleService) GetScheduleForTeacher(ctx context.Context, teacherName string, date time.Time) ([]domain.Lesson, error) {
	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}
	return s.lessonsForDate(ctx, lessons, date)
}

func (s *ScheduleService) GetScheduleForTeacherRange(ctx context.Context, teacherName string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}
	return s.lessonsForRange(ctx, lessons, from, to)
}

func (s *ScheduleService) GetScheduleForRoom(ctx context.Context, room string, date time.Time) ([]domain.Lesson, error) {
	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return s.lessonsForDate(ctx, lessons, date)
}

func (s *ScheduleService) GetScheduleForRoomRange(ctx context.Context, room string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}
	return s.lessonsForRange(ctx, lessons, from, to)
}

func (s *ScheduleService) lessonsForDate(ctx context.Context, lessons []domain.Lesson, date time.Time) ([]domain.Lesson, error) {
	date = helpers.NormalizeDate(date)
	semesterCache, err := s.buildSemesterCacheForLessons(ctx, lessons)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Lesson, 0)
	for _, lesson := range lessons {
		if lessonMatchesDate(lesson, date, semesterStart(semesterCache, lesson.SemesterID)) {
			result = append(result, lesson)
		}
	}
	sortLessons(result)
	return result, nil
}

func (s *ScheduleService) lessonsForRange(ctx context.Context, lessons []domain.Lesson, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	from = helpers.NormalizeDate(from)
	to = helpers.NormalizeDate(to)
	if to.Before(from) {
		return nil, fmt.Errorf("invalid schedule range")
	}
	semesterCache, err := s.buildSemesterCacheForLessons(ctx, lessons)
	if err != nil {
		return nil, err
	}
	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		for _, lesson := range lessons {
			if lessonMatchesDate(lesson, date, semesterStart(semesterCache, lesson.SemesterID)) {
				result[date] = append(result[date], lesson)
			}
		}
		sortLessons(result[date])
		if result[date] == nil {
			result[date] = []domain.Lesson{}
		}
	}
	return result, nil
}

func lessonMatchesDate(lesson domain.Lesson, date time.Time, fallbackSemesterStart *time.Time) bool {
	date = helpers.NormalizeDate(date)
	if lesson.SpecialDate != nil {
		return helpers.NormalizeDate(*lesson.SpecialDate).Equal(date)
	}
	if lesson.DayOfWeek != helpers.Weekday(date) {
		return false
	}

	if lesson.ValidFrom != nil {
		validFrom := helpers.NormalizeDate(*lesson.ValidFrom)
		if date.Before(validFrom) {
			return false
		}
		if lesson.ValidTo != nil && date.After(helpers.NormalizeDate(*lesson.ValidTo)) {
			return false
		}
		switch lesson.WeekType {
		case domain.WeekTypeOdd, domain.WeekTypeEven:
			days := int(date.Sub(validFrom).Hours() / 24)
			return days >= 0 && days%14 == 0
		case domain.WeekTypeEvery:
			return true
		}
	}
	if lesson.ValidTo != nil && date.After(helpers.NormalizeDate(*lesson.ValidTo)) {
		return false
	}
	if fallbackSemesterStart == nil {
		return lesson.WeekType == domain.WeekTypeEvery
	}
	weekType := helpers.DetermineWeekType(date, *fallbackSemesterStart)
	return helpers.MatchesWeekType(lesson.WeekType, weekType)
}

func semesterStart(cache map[string]*domain.Semester, semesterID string) *time.Time {
	semester := cache[semesterID]
	if semester == nil {
		return nil
	}
	return &semester.StartDate
}

func sortLessons(lessons []domain.Lesson) {
	sort.SliceStable(lessons, func(i, j int) bool {
		if lessons[i].TimeStart == lessons[j].TimeStart {
			return lessons[i].Subject < lessons[j].Subject
		}
		return lessons[i].TimeStart < lessons[j].TimeStart
	})
}

func (s *ScheduleService) buildSemesterCacheForLessons(ctx context.Context, lessons []domain.Lesson) (map[string]*domain.Semester, error) {
	unique := make(map[string]bool, len(lessons))
	for _, lesson := range lessons {
		if lesson.SemesterID != "" {
			unique[lesson.SemesterID] = true
		}
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	semesters, err := s.semesterRepo.GetSemestersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	cache := make(map[string]*domain.Semester, len(semesters))
	for i := range semesters {
		cache[semesters[i].ID] = &semesters[i]
	}
	return cache, nil
}

func (s *ScheduleService) ReplaceGroupLessons(ctx context.Context, groupID string, lessons []domain.Lesson) error {
	return s.lessonRepo.ReplaceLessonsForGroup(ctx, groupID, lessons)
}

func (s *ScheduleService) GetAllLessonsForGroup(ctx context.Context, groupID string) ([]domain.Lesson, error) {
	return s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
}

type ScheduleDiff struct {
	Added   int
	Removed int
}

func (d ScheduleDiff) Changed() bool {
	return d.Added > 0 || d.Removed > 0
}

// CompareLessonSnapshots compares schedule content, deliberately ignoring
// database IDs and updated_at. Source sites may regenerate identifiers even
// when the schedule itself has not changed.
func CompareLessonSnapshots(before, after []domain.Lesson) ScheduleDiff {
	beforeSet := lessonFingerprintCounts(before)
	afterSet := lessonFingerprintCounts(after)
	var diff ScheduleDiff
	for fingerprint, count := range afterSet {
		if delta := count - beforeSet[fingerprint]; delta > 0 {
			diff.Added += delta
		}
	}
	for fingerprint, count := range beforeSet {
		if delta := count - afterSet[fingerprint]; delta > 0 {
			diff.Removed += delta
		}
	}
	return diff
}

func lessonFingerprintCounts(lessons []domain.Lesson) map[string]int {
	result := make(map[string]int, len(lessons))
	for _, lesson := range lessons {
		payload := struct {
			UniversityID string
			SemesterID   string
			DayOfWeek    int
			SpecialDate  string
			TimeStart    string
			TimeEnd      string
			WeekType     domain.WeekType
			Subject      string
			Type         domain.LessonType
			Teacher      string
			Room         string
			GroupID      string
			Subgroup     int
			ValidFrom    string
			ValidTo      string
		}{
			UniversityID: lesson.UniversityID,
			SemesterID:   lesson.SemesterID,
			DayOfWeek:    lesson.DayOfWeek,
			SpecialDate:  fingerprintDate(lesson.SpecialDate),
			TimeStart:    lesson.TimeStart,
			TimeEnd:      lesson.TimeEnd,
			WeekType:     lesson.WeekType,
			Subject:      lesson.Subject,
			Type:         lesson.Type,
			Teacher:      lesson.Teacher,
			Room:         lesson.Room,
			GroupID:      lesson.GroupID,
			Subgroup:     lesson.Subgroup,
			ValidFrom:    fingerprintDate(lesson.ValidFrom),
			ValidTo:      fingerprintDate(lesson.ValidTo),
		}
		encoded, _ := json.Marshal(payload)
		result[string(encoded)]++
	}
	return result
}

func fingerprintDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *ScheduleService) SaveLessonsBatch(ctx context.Context, lessons []domain.Lesson) error {
	byGroup := make(map[string][]domain.Lesson)
	for _, lesson := range lessons {
		byGroup[lesson.GroupID] = append(byGroup[lesson.GroupID], lesson)
	}
	for groupID, groupLessons := range byGroup {
		if err := s.ReplaceGroupLessons(ctx, groupID, groupLessons); err != nil {
			return fmt.Errorf("save lessons batch group=%s: %w", groupID, err)
		}
	}
	return nil
}

func SplitMessage(text string, maxLen int) []string {
	if len([]rune(text)) <= maxLen {
		return []string{text}
	}
	var parts []string
	blocks := strings.Split(text, "\n\n")
	var current strings.Builder
	for _, block := range blocks {
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
