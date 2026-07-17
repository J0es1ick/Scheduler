package service

import (
	"context"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

type SemesterService struct {
	semesterRepo *repository.SemesterRepository
}

func NewSemesterService(semesterRepo *repository.SemesterRepository) *SemesterService {
	return &SemesterService{semesterRepo: semesterRepo}
}

// GetCurrentSemester возвращает семестр, в который попадает указанная дата.
func (s *SemesterService) GetCurrentSemester(ctx context.Context, universityID string, date time.Time) (*domain.Semester, error) {
	return s.semesterRepo.GetSemesterByDate(ctx, universityID, date)
}

// CreateSemester создаёт новый семестр.
func (s *SemesterService) CreateSemester(ctx context.Context, universityID, name string, startDate, endDate time.Time) error {
	id := uuid.New().String()
	_, err := s.semesterRepo.CreateSemester(ctx, id, universityID, name, startDate, endDate)
	return err
}

func (s *SemesterService) UpsertCurrentSnapshot(ctx context.Context, id, universityID string, startDate, endDate time.Time) error {
	return s.semesterRepo.UpsertSemester(ctx, id, universityID, "Актуальное расписание (авто)", startDate, endDate)
}
