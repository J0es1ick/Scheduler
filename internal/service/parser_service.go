package service

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scraper"
	"github.com/google/uuid"
)

type ParserService struct {
	dataSourceRepo *repository.DataSourceRepository
	parseLogRepo   *repository.ParseLogRepository
	scheduleSvc    *ScheduleService
	adapters       map[string]scraper.SourceAdapter // adapterType -> адаптер
}

func NewParserService(
	dataSourceRepo *repository.DataSourceRepository,
	parseLogRepo *repository.ParseLogRepository,
	scheduleSvc *ScheduleService,
) *ParserService {
	return &ParserService{
		dataSourceRepo: dataSourceRepo,
		parseLogRepo:   parseLogRepo,
		scheduleSvc:    scheduleSvc,
		adapters:       make(map[string]scraper.SourceAdapter),
	}
}

// RegisterAdapter регистрирует адаптер для типа источника.
func (s *ParserService) RegisterAdapter(adapterType string, adapter scraper.SourceAdapter) {
	s.adapters[adapterType] = adapter
}

// RunDataSource запускает парсинг для указанного источника данных.
// Возвращает количество полученных занятий и ошибку.
func (s *ParserService) RunDataSource(ctx context.Context, dataSourceID string) (int, error) {
	// Получаем источник
	ds, err := s.dataSourceRepo.GetDataSourceByID(ctx, dataSourceID)
	if err != nil || ds == nil {
		return 0, fmt.Errorf("data source not found: %w", err)
	}

	// Находим адаптер
	adapter, ok := s.adapters[ds.AdapterType]
	if !ok {
		return 0, fmt.Errorf("adapter not registered for type %s", ds.AdapterType)
	}

	// Логируем начало
	logID := uuid.New().String()
	_, err = s.parseLogRepo.CreateParseLog(ctx, logID, ds.ID, "running", 0, "")
	if err != nil {
		return 0, err
	}

	// Засекаем время
	start := time.Now()

	// Предположим, что адаптер сам знает, откуда брать расписание.
	lessons, err := adapter.FetchSchedule(ctx, "", time.Time{}, time.Time{}) // упрощённо, пока чисто нативная реализация
	if err != nil {
		s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", 0, err.Error())
		return 0, err
	}

	// Сохраняем расписание
	err = s.scheduleSvc.SaveLessonsBatch(ctx, lessons)
	if err != nil {
		s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", len(lessons), err.Error())
		return 0, err
	}

	// Обновляем лог успеха
	duration := time.Since(start)
	_ = duration // можно записать в отдельное поле, если добавить
	err = s.parseLogRepo.UpdateParseLog(ctx, logID, "success", len(lessons), "")
	if err != nil {
		return len(lessons), err
	}

	// Обновляем last_run_at в источнике
	ds.LastRunAt = time.Now()
	ds.LastError = ""
	_ = s.dataSourceRepo.UpdateDataSource(ctx, ds)

	return len(lessons), nil
}

// RunAllActiveSources запускает парсинг для всех активных источников.
func (s *ParserService) RunAllActiveSources(ctx context.Context) error {
	sources, err := s.dataSourceRepo.ListActiveDataSources(ctx)
	if err != nil {
		return err
	}
	for _, ds := range sources {
		_, err := s.RunDataSource(ctx, ds.ID)
		if err != nil {
			return err
		}
	}
	return nil
}