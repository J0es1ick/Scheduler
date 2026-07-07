package service

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

// UniversityService — слой бизнес-логики для работы с университетами.
// Он изолирует handler от прямого доступа к базе данных.
type UniversityService struct {
	repo *repository.UniversityRepository // репозиторий для работы с таблицей universities
}

// NewUniversityService — конструктор сервиса университетов.
// Используется для внедрения зависимости (repository).
func NewUniversityService(repo *repository.UniversityRepository) *UniversityService {
	return &UniversityService{repo: repo}
}

// GetAll возвращает список всех университетов из базы данных.
// Используется, например, для построения клавиатуры выбора университета в Telegram.
func (s *UniversityService) GetAll(ctx context.Context) ([]domain.University, error) {
	return s.repo.GetAllUniversities(ctx)
}

// GetByID возвращает университет по его ID (slug).
// Если университет не найден — вернётся nil без ошибки (зависит от репозитория).
func (s *UniversityService) GetByID(ctx context.Context, id string) (*domain.University, error) {
	return s.repo.GetUniversityByID(ctx, id)
}
