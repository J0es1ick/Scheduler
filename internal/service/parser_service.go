package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/google/uuid"
)

// ParserService управляет жизненным циклом адаптеров:
// регистрирует их, запускает парсинг и сохраняет результаты в БД.
//
// Схема работы:
//  1. RegisterAdapter вызывается один раз при старте (в main.go).
//  2. RunDataSource запускается планировщиком периодически.
//  3. RunDataSource → резолвим текущий семестр → FetchGroups → upsert групп в БД.
//  4. RunDataSource → FetchSchedule для каждой группы → SaveLessonsBatch.
type ParserService struct {
	dataSourceRepo *repository.DataSourceRepository
	parseLogRepo   *repository.ParseLogRepository
	groupRepo      *repository.GroupRepository
	scheduleSvc    *ScheduleService
	semesterSvc    *SemesterService
	adapters       map[string]scrapper.SourceAdapter // adapterType → адаптер
}

// NewParserService создаёт ParserService.
func NewParserService(
	dataSourceRepo *repository.DataSourceRepository,
	parseLogRepo *repository.ParseLogRepository,
	groupRepo *repository.GroupRepository,
	scheduleSvc *ScheduleService,
	semesterSvc *SemesterService,
) *ParserService {
	return &ParserService{
		dataSourceRepo: dataSourceRepo,
		parseLogRepo:   parseLogRepo,
		groupRepo:      groupRepo,
		scheduleSvc:    scheduleSvc,
		semesterSvc:    semesterSvc,
		adapters:       make(map[string]scrapper.SourceAdapter),
	}
}

// RegisterAdapter регистрирует адаптер по типу источника.
// adapterType должен совпадать с полем DataSource.AdapterType в БД.
func (s *ParserService) RegisterAdapter(adapterType string, adapter scrapper.SourceAdapter) {
	s.adapters[adapterType] = adapter
}

// RunDataSource запускает полный цикл парсинга для одного источника данных.
func (s *ParserService) RunDataSource(ctx context.Context, dataSourceID string) (int, error) {
	// 1. Получаем источник данных.
	ds, err := s.dataSourceRepo.GetDataSourceByID(ctx, dataSourceID)
	if err != nil {
		return 0, fmt.Errorf("parser: get data source: %w", err)
	}
	if ds == nil {
		return 0, fmt.Errorf("parser: data source %s not found", dataSourceID)
	}

	// 2. Находим адаптер.
	adapter, ok := s.adapters[ds.AdapterType]
	if !ok {
		return 0, fmt.Errorf("parser: no adapter registered for type=%q", ds.AdapterType)
	}

	// 3. Динамически резолвим текущий семестр.
	// Делаем это при каждом запуске, а не один раз при старте приложения —
	// так адаптер всегда получает актуальный semesterID даже после смены семестра.
	semester, err := s.semesterSvc.GetCurrentSemester(ctx, adapter.UniversityID(), time.Now())
	if err != nil {
		return 0, fmt.Errorf("parser: get current semester for %s: %w", adapter.UniversityID(), err)
	}
	if semester == nil {
		return 0, fmt.Errorf("parser: нет активного семестра для %s — добавьте запись в таблицу semesters", adapter.UniversityID())
	}
	adapter.SetSemesterID(semester.ID)
	slog.Info("parser: semester resolved", "adapter", adapter.Name(), "semester", semester.Name)

	// 4. Создаём лог.
	logID := uuid.New().String()
	if _, err = s.parseLogRepo.CreateParseLog(ctx, logID, ds.ID, "running", 0, ""); err != nil {
		return 0, fmt.Errorf("parser: create parse log: %w", err)
	}

	start := time.Now()
	totalLessons := 0

	// 5. Загружаем группы.
	groups, err := adapter.FetchGroups(ctx)
	if err != nil {
		_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", 0, err.Error())
		_ = s.markDataSourceError(ctx, ds, err.Error())
		return 0, fmt.Errorf("parser: FetchGroups [%s]: %w", adapter.Name(), err)
	}

	// Upsert групп в БД.
	for _, g := range groups {
		if upsertErr := s.upsertGroup(ctx, g); upsertErr != nil {
			slog.Warn("parser: upsert group failed",
				"group", g.Name, "err", upsertErr)
		}
	}
	slog.Info("parser: groups synced", "adapter", adapter.Name(), "count", len(groups))

	// 6. Загружаем расписание для каждой группы.
	for _, g := range groups {
		select {
		case <-ctx.Done():
			_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", totalLessons, "context cancelled")
			return totalLessons, ctx.Err()
		default:
		}

		lessons, fetchErr := adapter.FetchSchedule(ctx, g.ID)
		if fetchErr != nil {
			slog.Warn("parser: FetchSchedule failed",
				"adapter", adapter.Name(), "group", g.Name, "err", fetchErr)
			continue
		}

		if saveErr := s.scheduleSvc.SaveLessonsBatch(ctx, lessons); saveErr != nil {
			slog.Warn("parser: SaveLessonsBatch failed",
				"adapter", adapter.Name(), "group", g.Name, "err", saveErr)
			continue
		}
		totalLessons += len(lessons)
	}

	// 7. Обновляем лог и источник данных.
	elapsed := time.Since(start)
	slog.Info("parser: data source run complete",
		"adapter", adapter.Name(), "lessons", totalLessons, "elapsed", elapsed)

	_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "success", totalLessons, "")
	_ = s.markDataSourceSuccess(ctx, ds)

	return totalLessons, nil
}

// RunAllActiveSources запускает парсинг для всех источников, которым пришло время обновиться.
func (s *ParserService) RunAllActiveSources(ctx context.Context) error {
	sources, err := s.dataSourceRepo.ListActiveDataSources(ctx)
	if err != nil {
		return fmt.Errorf("parser: list active sources: %w", err)
	}

	if len(sources) == 0 {
		slog.Debug("parser: no active sources to run")
		return nil
	}

	var lastErr error
	for _, ds := range sources {
		if _, err := s.RunDataSource(ctx, ds.ID); err != nil {
			slog.Error("parser: RunDataSource error",
				"dataSourceID", ds.ID,
				"adapterType", ds.AdapterType,
				"err", err)
			lastErr = err
		}
	}
	return lastErr
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func (s *ParserService) upsertGroup(ctx context.Context, g domain.Group) error {
	existing, err := s.groupRepo.GetGroupByID(ctx, g.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		_, err = s.groupRepo.CreateGroup(ctx, g.ID, g.UniversityID, g.Name, g.IsActive)
		return err
	}
	if existing.Name != g.Name {
		return s.groupRepo.UpdateGroup(ctx, g.ID, g.Name, g.IsActive)
	}
	return nil
}

func (s *ParserService) markDataSourceError(ctx context.Context, ds *domain.DataSource, errMsg string) error {
	ds.LastRunAt = time.Now()
	ds.LastError = errMsg
	return s.dataSourceRepo.UpdateDataSource(ctx, ds)
}

func (s *ParserService) markDataSourceSuccess(ctx context.Context, ds *domain.DataSource) error {
	ds.LastRunAt = time.Now()
	ds.LastError = ""
	return s.dataSourceRepo.UpdateDataSource(ctx, ds)
}