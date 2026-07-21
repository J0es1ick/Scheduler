package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/google/uuid"
)

const scheduleFetchConcurrency = 3

var ErrDataSourceBusy = errors.New("parser: data source is already running")

type ParserService struct {
	dataSourceRepo   *repository.DataSourceRepository
	parseLogRepo     *repository.ParseLogRepository
	groupRepo        *repository.GroupRepository
	scheduleSvc      *ScheduleService
	semesterSvc      *SemesterService
	notificationRepo *repository.NotificationRepository
	adapters         map[string]scrapper.SourceAdapter
}

func NewParserService(
	dataSourceRepo *repository.DataSourceRepository,
	parseLogRepo *repository.ParseLogRepository,
	groupRepo *repository.GroupRepository,
	scheduleSvc *ScheduleService,
	semesterSvc *SemesterService,
	notificationRepo *repository.NotificationRepository,
) *ParserService {
	return &ParserService{
		dataSourceRepo:   dataSourceRepo,
		parseLogRepo:     parseLogRepo,
		groupRepo:        groupRepo,
		scheduleSvc:      scheduleSvc,
		semesterSvc:      semesterSvc,
		notificationRepo: notificationRepo,
		adapters:         make(map[string]scrapper.SourceAdapter),
	}
}

func (s *ParserService) RegisterAdapter(adapterType string, adapter scrapper.SourceAdapter) {
	s.adapters[adapterType] = adapter
}

func (s *ParserService) RunDataSource(ctx context.Context, dataSourceID string) (int, error) {
	release, acquired, err := s.dataSourceRepo.TryAcquireRunLock(ctx, dataSourceID)
	if err != nil {
		return 0, err
	}
	if !acquired {
		return 0, fmt.Errorf("%w: %s", ErrDataSourceBusy, dataSourceID)
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			slog.Error("parser: release data source lock failed", "dataSourceID", dataSourceID, "err", releaseErr)
		}
	}()

	ds, err := s.dataSourceRepo.GetDataSourceByID(ctx, dataSourceID)
	if err != nil {
		return 0, fmt.Errorf("parser: get data source: %w", err)
	}
	if ds == nil {
		return 0, fmt.Errorf("parser: data source %s not found", dataSourceID)
	}
	adapter, ok := s.adapters[ds.AdapterType]
	if !ok {
		return 0, fmt.Errorf("parser: no adapter registered for type=%q", ds.AdapterType)
	}

	semesterID := adapter.UniversityID() + "-current"
	adapter.SetSemesterID(semesterID)

	logID := uuid.New().String()
	if _, err = s.parseLogRepo.CreateParseLog(ctx, logID, ds.ID, "running", 0, ""); err != nil {
		return 0, fmt.Errorf("parser: create parse log: %w", err)
	}
	startedAt := time.Now()
	fail := func(records int, runErr error) (int, error) {
		message := runErr.Error()
		_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", records, message)
		_ = s.markDataSourceError(ctx, ds, message)
		return records, runErr
	}

	groups, err := adapter.FetchGroups(ctx)
	if err != nil {
		return fail(0, fmt.Errorf("parser: FetchGroups [%s]: %w", adapter.Name(), err))
	}
	if len(groups) == 0 {
		return fail(0, fmt.Errorf("parser: FetchGroups [%s] returned no groups", adapter.Name()))
	}

	activeIDs := make([]string, 0, len(groups))
	for _, group := range groups {
		if err = s.upsertGroup(ctx, group); err != nil {
			return fail(0, fmt.Errorf("parser: sync group %s: %w", group.Name, err))
		}
		activeIDs = append(activeIDs, group.ID)
	}
	if err = s.groupRepo.DeactivateGroupsExcept(ctx, adapter.UniversityID(), activeIDs); err != nil {
		return fail(0, fmt.Errorf("parser: deactivate stale groups: %w", err))
	}
	slog.Info("parser: groups synced", "adapter", adapter.Name(), "count", len(groups))

	results := s.fetchSchedules(ctx, adapter, groups)
	var fetchErrors []error
	minDate, maxDate := time.Time{}, time.Time{}
	successfulGroups := 0
	for _, result := range results {
		if result.err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("group %s: %w", result.group.Name, result.err))
			continue
		}
		successfulGroups++
		for _, lesson := range result.lessons {
			if lesson.ValidFrom != nil && (minDate.IsZero() || lesson.ValidFrom.Before(minDate)) {
				minDate = *lesson.ValidFrom
			}
			if lesson.ValidTo != nil && (maxDate.IsZero() || lesson.ValidTo.After(maxDate)) {
				maxDate = *lesson.ValidTo
			}
		}
	}
	if successfulGroups == 0 {
		return fail(0, fmt.Errorf("parser: all %d schedule requests failed: %w", len(groups), errors.Join(fetchErrors...)))
	}
	if minDate.IsZero() {
		minDate = time.Now()
	}
	if maxDate.IsZero() {
		maxDate = minDate
	}
	if err = s.semesterSvc.UpsertCurrentSnapshot(ctx, semesterID, adapter.UniversityID(), minDate, maxDate); err != nil {
		return fail(0, fmt.Errorf("parser: publish current semester metadata: %w", err))
	}

	totalLessons := 0
	var saveErrors []error
	for _, result := range results {
		if result.err != nil {
			continue // keep the previous schedule for a group that could not be fetched
		}
		for i := range result.lessons {
			result.lessons[i].SemesterID = semesterID
		}
		before, snapshotErr := s.scheduleSvc.GetAllLessonsForGroup(ctx, result.group.ID)
		if snapshotErr != nil {
			saveErrors = append(saveErrors, fmt.Errorf("group %s: read previous schedule: %w", result.group.Name, snapshotErr))
			continue
		}
		if err = s.scheduleSvc.ReplaceGroupLessons(ctx, result.group.ID, result.lessons); err != nil {
			saveErrors = append(saveErrors, fmt.Errorf("group %s: %w", result.group.Name, err))
			continue
		}
		totalLessons += len(result.lessons)
		after, snapshotErr := s.scheduleSvc.GetAllLessonsForGroup(ctx, result.group.ID)
		if snapshotErr != nil {
			saveErrors = append(saveErrors, fmt.Errorf("group %s: read published schedule: %w", result.group.Name, snapshotErr))
			continue
		}
		diff := CompareLessonSnapshots(before, after)
		if diff.Changed() && s.notificationRepo != nil {
			if enqueueErr := s.notificationRepo.EnqueueScheduleChange(
				ctx,
				uuid.NewString(),
				result.group.ID,
				"parser",
				scheduleChangeSummary(diff),
			); enqueueErr != nil {
				slog.Error("parser: enqueue schedule notification failed",
					"group", result.group.ID,
					"err", enqueueErr,
				)
			}
		}
	}

	combinedErrors := append(fetchErrors, saveErrors...)
	if len(combinedErrors) > 0 {
		runErr := fmt.Errorf("parser: incomplete snapshot (%d fetch errors, %d save errors): %w", len(fetchErrors), len(saveErrors), errors.Join(combinedErrors...))
		return fail(totalLessons, runErr)
	}

	slog.Info("parser: data source run complete",
		"adapter", adapter.Name(),
		"groups", len(groups),
		"lessons", totalLessons,
		"elapsed", time.Since(startedAt),
	)
	_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "success", totalLessons, "")
	_ = s.markDataSourceSuccess(ctx, ds)
	return totalLessons, nil
}

func scheduleChangeSummary(diff ScheduleDiff) string {
	modified := min(diff.Added, diff.Removed)
	added := diff.Added - modified
	removed := diff.Removed - modified
	parts := make([]string, 0, 3)
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("изменено: %d", modified))
	}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("добавлено: %d", added))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("удалено: %d", removed))
	}
	return "Расписание обновлено — " + strings.Join(parts, ", ") + "."
}

type groupScheduleResult struct {
	group   domain.Group
	lessons []domain.Lesson
	err     error
}

func (s *ParserService) fetchSchedules(ctx context.Context, adapter scrapper.SourceAdapter, groups []domain.Group) []groupScheduleResult {
	results := make([]groupScheduleResult, len(groups))
	sem := make(chan struct{}, scheduleFetchConcurrency)
	var wg sync.WaitGroup
	for i, group := range groups {
		i, group := i, group
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = groupScheduleResult{group: group, err: ctx.Err()}
				return
			}
			lessons, err := adapter.FetchSchedule(ctx, group.ID)
			results[i] = groupScheduleResult{group: group, lessons: lessons, err: err}
		}()
	}
	wg.Wait()
	return results
}

func (s *ParserService) RunAllActiveSources(ctx context.Context) error {
	sources, err := s.dataSourceRepo.ListActiveDataSources(ctx)
	if err != nil {
		return fmt.Errorf("parser: list active sources: %w", err)
	}
	var runErrors []error
	for _, dataSource := range sources {
		if _, err = s.RunDataSource(ctx, dataSource.ID); err != nil {
			slog.Error("parser: source run failed", "dataSourceID", dataSource.ID, "err", err)
			runErrors = append(runErrors, err)
		}
	}
	return errors.Join(runErrors...)
}

func (s *ParserService) upsertGroup(ctx context.Context, group domain.Group) error {
	existing, err := s.groupRepo.GetGroupByID(ctx, group.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		_, err = s.groupRepo.CreateGroup(ctx, group.ID, group.UniversityID, group.Name, true)
		return err
	}
	if existing.Name != group.Name || !existing.IsActive {
		return s.groupRepo.UpdateGroup(ctx, group.ID, group.Name, true)
	}
	return nil
}

func (s *ParserService) markDataSourceError(ctx context.Context, dataSource *domain.DataSource, message string) error {
	dataSource.LastRunAt = time.Now()
	dataSource.LastError = message
	return s.dataSourceRepo.UpdateDataSource(ctx, dataSource)
}

func (s *ParserService) markDataSourceSuccess(ctx context.Context, dataSource *domain.DataSource) error {
	dataSource.LastRunAt = time.Now()
	dataSource.LastError = ""
	return s.dataSourceRepo.UpdateDataSource(ctx, dataSource)
}
