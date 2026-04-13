package service

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
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

// GetScheduleForGroup возвращает расписание для группы на конкретную дату.
// Работает с двумя типами занятий:
// - шаблонные (special_date IS NULL) – проверяются day_of_week + week_type
// - конкретные (special_date = дата) – берутся напрямую
func (s *ScheduleService) GetScheduleForGroup(ctx context.Context, groupID string, date time.Time) ([]domain.Lesson, error) {
	// Получаем группу, чтобы узнать universityID
	group, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("group not found")
	}

	// Определяем семестр для даты
	semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
	if err != nil {
		return nil, err
	}
	if semester == nil {
		return nil, fmt.Errorf("no semester found for date %v", date)
	}

	// Получаем все занятия группы в этом семестре (оптимизировано одним запросом)
	allLessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	// Фильтруем по семестру и дате
	var result []domain.Lesson
	weekType := determineWeekType(date, semester.StartDate) // odd/even
	dayOfWeek := int(date.Weekday()) // 0=воскресенье, преобразуем в 1-7

	for _, lesson := range allLessons {
		if lesson.SemesterID != semester.ID {
			continue
		}
		// Конкретное занятие на эту дату
		if lesson.SpecialDate != nil && lesson.SpecialDate.Equal(date) {
			result = append(result, lesson)
			continue
		}
		// Шаблонное занятие: проверяем день недели и тип недели
		if lesson.SpecialDate == nil {
			if lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
				result = append(result, lesson)
			}
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForGroupRange(ctx context.Context, groupID string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
    group, err := s.groupRepo.GetGroupByID(ctx, groupID)
    if err != nil || group == nil {
        return nil, fmt.Errorf("group not found")
    }

    // Определяем семестры, которые пересекаются с диапазоном
    semesters, err := s.semesterRepo.GetSemestersByUniversityID(ctx, group.UniversityID)
    if err != nil {
        return nil, err
    }

    // Получаем все занятия группы (шаблонные и конкретные)
    allLessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
    if err != nil {
        return nil, err
    }

    // Фильтруем по семестрам и датам
    result := make(map[time.Time][]domain.Lesson)
    for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
        result[date] = []domain.Lesson{}
    }

    for _, lesson := range allLessons {
        // Проверяем, попадает ли занятие в нужный семестр
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

        // Конкретное занятие
        if lesson.SpecialDate != nil {
            if _, ok := result[*lesson.SpecialDate]; ok {
                result[*lesson.SpecialDate] = append(result[*lesson.SpecialDate], lesson)
            }
            continue
        }

        // Шаблонное занятие – разворачиваем на все даты диапазона
        for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
            dayOfWeek := int(date.Weekday()) // приведение к 1..7
            weekType := determineWeekType(date, semester.StartDate)
            if lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
                result[date] = append(result[date], lesson)
            }
        }
    }
    return result, nil
}

func (s *ScheduleService) GetScheduleForTeacher(ctx context.Context, teacherName string, date time.Time) ([]domain.Lesson, error) {
	// Получаем все занятия преподавателя (по имени, может быть несколько)
	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}

	// Фильтруем по дате (аналогично методу для группы)
	var result []domain.Lesson
	for _, lesson := range lessons {
		// Получаем группу, чтобы узнать universityID
		group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
		if err != nil || group == nil {
			continue
		}
		// Определяем семестр для даты
		semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
		if err != nil || semester == nil {
			continue
		}
		weekType := determineWeekType(date, semester.StartDate)
		dayOfWeek := int(date.Weekday())
		if lesson.SpecialDate != nil && lesson.SpecialDate.Equal(date) {
			result = append(result, lesson)
		}
		if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
			result = append(result, lesson)
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForTeacherRange(ctx context.Context, teacherName string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	// Получаем все занятия преподавателя (по имени, может быть несколько)
	lessons, err := s.lessonRepo.GetLessonsByTeacher(ctx, teacherName)
	if err != nil {
		return nil, err
	}

	// Фильтруем по дате (аналогично методу для группы)
	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result[date] = []domain.Lesson{}
	}

	date := from
	for !date.After(to) {
			dayOfWeek := int(date.Weekday())
			for _, lesson := range lessons {
				// Получаем группу, чтобы узнать universityID
				group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
				if err != nil || group == nil {
					continue
				}
				// Определяем семестр для даты
				semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
				if err != nil || semester == nil {
					continue
				}
				weekType := determineWeekType(date, semester.StartDate)
				if lesson.SpecialDate != nil && lesson.SpecialDate.Equal(date) {
					result[date] = append(result[date], lesson)
				}
				if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
					result[date] = append(result[date], lesson)
				}
			}
			date = date.AddDate(0, 0, 1)
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForRoom(ctx context.Context, room string, date time.Time) ([]domain.Lesson, error) {
	// Получаем все занятия в аудитории
	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}

	// Фильтруем по дате (аналогично методу для группы)
	var result []domain.Lesson
	for _, lesson := range lessons {
		// Получаем группу, чтобы узнать universityID
		group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
		if err != nil || group == nil {
			continue
		}
		// Определяем семестр для даты
		semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
		if err != nil || semester == nil {
			continue
		}
		weekType := determineWeekType(date, semester.StartDate)
		dayOfWeek := int(date.Weekday())
		if lesson.SpecialDate != nil && lesson.SpecialDate.Equal(date) {
			result = append(result, lesson)
		}
		if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
			result = append(result, lesson)
		}
	}
	return result, nil
}

func (s *ScheduleService) GetScheduleForRoomRange(ctx context.Context, room string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	// Получаем все занятия в аудитории
	lessons, err := s.lessonRepo.GetLessonsByRoom(ctx, room)
	if err != nil {
		return nil, err
	}

	// Фильтруем по дате (аналогично методу для группы)
	result := make(map[time.Time][]domain.Lesson)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result[date] = []domain.Lesson{}
	}

	date := from
	for !date.After(to) {
			dayOfWeek := int(date.Weekday())
			for _, lesson := range lessons {
				// Получаем группу, чтобы узнать universityID
				group, err := s.groupRepo.GetGroupByID(ctx, lesson.GroupID)
				if err != nil || group == nil {
					continue
				}
				// Определяем семестр для даты
				semester, err := s.semesterRepo.GetSemesterByDate(ctx, group.UniversityID, date)
				if err != nil || semester == nil {
					continue
				}
				weekType := determineWeekType(date, semester.StartDate)
				if lesson.SpecialDate != nil && lesson.SpecialDate.Equal(date) {
					result[date] = append(result[date], lesson)
				}
				if lesson.SpecialDate == nil && lesson.DayOfWeek == dayOfWeek && matchesWeekType(lesson.WeekType, weekType) {
					result[date] = append(result[date], lesson)
				}
			}
			date = date.AddDate(0, 0, 1)
	}
	return result, nil
}

// SaveLessonsBatch сохраняет список занятий (для использования парсерами).
// Сначала удаляет старые занятия за тот же семестр и группу (или по датам).
func (s *ScheduleService) SaveLessonsBatch(ctx context.Context, lessons []domain.Lesson) error {
	// Группируем по семестру и группе для очистки
	type key struct{ semesterID, groupID string }
	keysMap := make(map[key]bool)
	for _, l := range lessons {
		keysMap[key{l.SemesterID, l.GroupID}] = true
	}

	// Удаляем старые записи (можно реализовать метод в LessonRepository: DeleteBySemesterAndGroup)
	// Пока заглушка – для каждого ключа удаляем вручную.
	for k := range keysMap {
		// Здесь нужен отдельный метод репозитория, например:
		// err := s.lessonRepo.DeleteBySemesterAndGroup(ctx, k.semesterID, k.groupID)
		// if err != nil { return err }
		_ = k
	}

	// Вставляем новые
	for _, lesson := range lessons {
		_, err := s.lessonRepo.CreateLesson(ctx, lesson)
		if err != nil {
			return fmt.Errorf("failed to insert lesson: %w", err)
		}
	}
	return nil
}

// вспомогательные функции
func determineWeekType(date, semesterStart time.Time) domain.WeekType {
	daysDiff := int(date.Sub(semesterStart).Hours() / 24)
	weekNumber := daysDiff/7 + 1
	if weekNumber%2 == 1 {
		return domain.WeekTypeOdd
	}
	return domain.WeekTypeEven
}

func matchesWeekType(lessonWeekType, currentWeekType domain.WeekType) bool {
	if lessonWeekType == domain.WeekTypeEvery {
		return true
	}
	return lessonWeekType == currentWeekType
}