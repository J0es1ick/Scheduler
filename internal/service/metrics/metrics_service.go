package metrics

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
	operationsrepo "github.com/J0es1ick/Scheduler/internal/repository/operations"
)

type MetricsService struct {
	repo *operationsrepo.MetricsRepository
}

func NewMetricsService(repo *operationsrepo.MetricsRepository) *MetricsService {
	return &MetricsService{repo: repo}
}

func (s *MetricsService) Get(ctx context.Context) (*domain.ServiceMetrics, error) {
	return s.repo.Get(ctx)
}
