package service

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

type MetricsService struct {
	repo *repository.MetricsRepository
}

func NewMetricsService(repo *repository.MetricsRepository) *MetricsService {
	return &MetricsService{repo: repo}
}

func (s *MetricsService) Get(ctx context.Context) (*domain.ServiceMetrics, error) {
	return s.repo.Get(ctx)
}
