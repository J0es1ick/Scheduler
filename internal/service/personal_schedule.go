package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
)

type PersonalLesson struct {
	Lesson      domain.Lesson             `json:"lesson"`
	Original    domain.Lesson             `json:"original"`
	Cancelled   bool                      `json:"cancelled"`
	Changes     []domain.PersonalOverride `json:"changes"`
	SemesterEnd string                    `json:"semester_end"`
	GroupName   string                    `json:"group_name"`
	CanRepeat   bool                      `json:"can_repeat"`
}

type PersonalDay struct {
	Date    string           `json:"date"`
	Lessons []PersonalLesson `json:"lessons"`
}
type PersonalSchedule struct {
	Target  domain.PersonalTarget     `json:"target"`
	Days    []PersonalDay             `json:"days"`
	Changes []domain.PersonalOverride `json:"changes"`
}

func (s *ScheduleService) PersonalTargets(ctx context.Context, userID string) ([]domain.PersonalTarget, error) {
	return s.personalRepo.Targets(ctx, userID)
}

func (s *ScheduleService) PersonalSchedule(ctx context.Context, userID, targetID string, from, to time.Time) (*PersonalSchedule, error) {
	targets, err := s.PersonalTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	var target *domain.PersonalTarget
	for i := range targets {
		if targets[i].ID == targetID {
			target = &targets[i]
			break
		}
	}
	if target == nil {
		return nil, sql.ErrNoRows
	}
	var data map[time.Time][]domain.Lesson
	if target.Role == domain.RoleTeacher {
		data, err = s.GetScheduleForTeacherRange(ctx, target.UniversityID, target.Name, from, to)
	} else {
		data, err = s.GetScheduleForGroupRange(ctx, target.ID, from, to)
	}
	if err != nil {
		return nil, err
	}
	changes, err := s.personalRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := &PersonalSchedule{Target: *target, Days: []PersonalDay{}, Changes: []domain.PersonalOverride{}}
	for _, change := range changes {
		if change.Role == target.Role && change.TargetID == target.ID {
			result.Changes = append(result.Changes, change)
		}
	}
	semesters := map[string]*domain.Semester{}
	for date := helpers.NormalizeDate(from); !date.After(helpers.NormalizeDate(to)); date = date.AddDate(0, 0, 1) {
		day := PersonalDay{Date: date.Format(time.DateOnly), Lessons: []PersonalLesson{}}
		for _, lesson := range data[date] {
			if target.Subgroup > 0 && lesson.Subgroup > 0 && lesson.Subgroup != target.Subgroup {
				continue
			}
			semester := semesters[lesson.SemesterID]
			if semester == nil {
				semester, err = s.personalRepo.Semester(ctx, lesson.SemesterID)
				if err != nil {
					return nil, err
				}
				semesters[lesson.SemesterID] = semester
			}
			item, applyErr := applyPersonalLesson(lesson, date, result.Changes)
			if applyErr != nil {
				return nil, applyErr
			}
			item.GroupName = lesson.GroupName
			item.SemesterEnd = semester.EndDate.Format(time.DateOnly)
			item.CanRepeat = repeatablePersonalLesson(lesson) && !semester.EndDate.Before(date)
			day.Lessons = append(day.Lessons, item)
		}
		result.Days = append(result.Days, day)
	}
	return result, nil
}

func applyPersonalLesson(lesson domain.Lesson, date time.Time, changes []domain.PersonalOverride) (PersonalLesson, error) {
	result := PersonalLesson{Lesson: lesson, Original: lesson, Changes: []domain.PersonalOverride{}}
	for _, change := range changes {
		if !change.Matches(lesson, date) {
			continue
		}
		var patch domain.PersonalLessonPatch
		if err := json.Unmarshal(change.Patch, &patch); err != nil {
			return result, err
		}
		result.Lesson = patch.Apply(result.Lesson)
		result.Cancelled = change.Cancelled
		result.Changes = append(result.Changes, change)
	}
	return result, nil
}

func (s *ScheduleService) PersonalizeSchedule(ctx context.Context, userID, groupID, universityID, teacher string, data map[time.Time][]domain.Lesson) (map[time.Time][]domain.Lesson, error) {
	if userID == "" {
		return data, nil
	}
	targets, err := s.PersonalTargets(ctx, userID)
	if err != nil {
		return nil, err
	}
	targetID := ""
	var role domain.UserRole
	for _, target := range targets {
		if (teacher == "" && target.Role == domain.RoleStudent && target.ID == groupID) || (teacher != "" && target.Role == domain.RoleTeacher && target.UniversityID == universityID && searchtext.TokenKey(target.Name) == searchtext.TokenKey(teacher)) {
			targetID = target.ID
			role = target.Role
			break
		}
	}
	if targetID == "" {
		return data, nil
	}
	changes, err := s.personalRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	applicable := []domain.PersonalOverride{}
	for _, change := range changes {
		if change.Role == role && change.TargetID == targetID {
			applicable = append(applicable, change)
		}
	}
	result := make(map[time.Time][]domain.Lesson, len(data))
	for date, lessons := range data {
		result[date] = []domain.Lesson{}
		for _, lesson := range lessons {
			item, err := applyPersonalLesson(lesson, date, applicable)
			if err != nil {
				return nil, err
			}
			if !item.Cancelled {
				result[date] = append(result[date], item.Lesson)
			}
		}
		sortLessons(result[date])
	}
	return result, nil
}

func repeatablePersonalLesson(lesson domain.Lesson) bool {
	return lesson.SpecialDate == nil && lesson.WeekType != domain.WeekTypeDate && lesson.Recurrence.CycleLength <= 2
}

type PersonalChangeInput struct {
	ID        string                     `json:"id"`
	Version   int64                      `json:"version"`
	TargetID  string                     `json:"target_id"`
	LessonID  string                     `json:"lesson_id"`
	Date      string                     `json:"date"`
	Scope     string                     `json:"scope"`
	Patch     domain.PersonalLessonPatch `json:"patch"`
	Cancelled bool                       `json:"cancelled"`
}

var ErrPersonalInput = errors.New("invalid personal schedule input")

func personalInput(message string) error { return fmt.Errorf("%w: %s", ErrPersonalInput, message) }

func (s *ScheduleService) SavePersonalChange(ctx context.Context, userID string, input PersonalChangeInput) (*domain.PersonalOverride, error) {
	date, err := time.Parse(time.DateOnly, input.Date)
	if err != nil || len(input.Date) != 10 {
		return nil, personalInput("Укажите дату занятия")
	}
	if input.Scope != "day" && input.Scope != "semester" {
		return nil, personalInput("Выберите срок действия правки")
	}
	schedule, err := s.PersonalSchedule(ctx, userID, input.TargetID, date, date)
	if err != nil {
		return nil, err
	}
	var lesson *domain.Lesson
	for _, item := range schedule.Days[0].Lessons {
		if item.Original.ID == input.LessonID || item.Original.PersonalKey == input.LessonID {
			base := item.Original
			lesson = &base
			break
		}
	}
	if lesson == nil {
		return nil, personalInput("Занятие изменилось или больше не опубликовано. Обновите расписание")
	}
	if err = validatePersonalPatch(input.Patch); err != nil {
		return nil, err
	}
	until := date
	if input.Scope == "semester" {
		if !repeatablePersonalLesson(*lesson) {
			return nil, personalInput("Для этого занятия доступна только правка конкретной даты")
		}
		semester, loadErr := s.personalRepo.Semester(ctx, lesson.SemesterID)
		if loadErr != nil {
			return nil, loadErr
		}
		until = semester.EndDate
		if until.Before(date) || date.Before(semester.StartDate) {
			return nil, personalInput("Дата находится за пределами семестра")
		}
	}
	raw, err := json.Marshal(input.Patch)
	if err != nil {
		return nil, err
	}

	if input.ID == "" && len(schedule.Changes) >= 2000 {
		return nil, personalInput("Слишком много личных правок. Удалите ненужные")
	}
	key := lesson.PersonalKey
	if key == "" {
		key = lesson.ID
	}
	item := domain.PersonalOverride{ID: input.ID, UserID: userID, Role: schedule.Target.Role, TargetID: input.TargetID, LessonID: key, UniversityID: lesson.UniversityID, SemesterID: lesson.SemesterID, Scope: input.Scope, ValidFrom: date, ValidTo: until, Patch: raw, Cancelled: input.Cancelled}
	return s.personalRepo.Save(ctx, item, input.Version)
}

func validatePersonalPatch(p domain.PersonalLessonPatch) error {
	for _, field := range []*string{p.Subject, p.Teacher, p.Room} {
		if field != nil {
			*field = strings.TrimSpace(*field)
			if utf8.RuneCountInString(*field) > 300 || strings.ContainsAny(*field, "\r\n\x00") {
				return personalInput("Поле должно содержать одну строку до 300 символов")
			}
		}
	}
	if p.Subject != nil && *p.Subject == "" {
		return personalInput("Введите название занятия")
	}
	if p.Type != nil {
		switch *p.Type {
		case domain.LessonTypeLecture, domain.LessonTypePractice, domain.LessonTypeLab, domain.LessonTypeSeminar, domain.LessonTypeExam, domain.LessonTypeCredit, domain.LessonTypeConsultation, domain.LessonTypeOther:
		default:
			return personalInput("Неизвестный тип занятия")
		}
	}
	if (p.TimeStart == nil) != (p.TimeEnd == nil) {
		return personalInput("Укажите начало и окончание занятия")
	}
	if p.TimeStart != nil {
		start, err := ParseDailyTime(*p.TimeStart)
		if err != nil {
			return personalInput("Неверное время начала")
		}
		end, err := ParseDailyTime(*p.TimeEnd)
		if err != nil || end <= start {
			return personalInput("Окончание должно быть позже начала")
		}
		*p.TimeStart = start
		*p.TimeEnd = end
	}
	return nil
}
