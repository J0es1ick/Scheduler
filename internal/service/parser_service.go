package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scrapper"
	"github.com/google/uuid"
)

const scheduleFetchConcurrency = 3
const scheduleFetchAttempts = 3

var (
	ErrDataSourceBusy     = errors.New("parser: data source is already running")
	ErrSnapshotQuarantine = errors.New("parser: candidate snapshot quarantined")
)

type ParserService struct {
	dataSourceRepo   *repository.DataSourceRepository
	parseLogRepo     *repository.ParseLogRepository
	groupRepo        *repository.GroupRepository
	scheduleSvc      *ScheduleService
	snapshotRepo     *repository.ParserSnapshotRepository
	notificationRepo *repository.NotificationRepository
	adapters         map[string]scrapper.SourceAdapter
}

func NewParserService(
	dataSourceRepo *repository.DataSourceRepository,
	parseLogRepo *repository.ParseLogRepository,
	groupRepo *repository.GroupRepository,
	scheduleSvc *ScheduleService,
	snapshotRepo *repository.ParserSnapshotRepository,
	notificationRepo *repository.NotificationRepository,
) *ParserService {
	return &ParserService{
		dataSourceRepo:   dataSourceRepo,
		parseLogRepo:     parseLogRepo,
		groupRepo:        groupRepo,
		scheduleSvc:      scheduleSvc,
		snapshotRepo:     snapshotRepo,
		notificationRepo: notificationRepo,
		adapters:         make(map[string]scrapper.SourceAdapter),
	}
}

func (s *ParserService) RegisterAdapter(adapterType string, adapter scrapper.SourceAdapter) {
	s.adapters[adapterType] = adapter
}

func (s *ParserService) CleanupInterruptedRuns(ctx context.Context, olderThan time.Duration) error {
	count, err := s.parseLogRepo.FailInterrupted(ctx, olderThan)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Warn("parser: interrupted runs marked failed", "count", count)
	}
	return nil
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
	logID := uuid.NewString()
	if _, err = s.parseLogRepo.CreateParseLog(ctx, logID, ds.ID, "running", 0, ""); err != nil {
		return 0, fmt.Errorf("parser: create parse log: %w", err)
	}
	startedAt := time.Now()
	fail := func(records int, runErr error) (int, error) {
		message := truncate(runErr.Error(), 4000)
		_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "failed", records, message)
		failures, recordErr := s.dataSourceRepo.RecordFailure(ctx, ds.ID, message)
		if recordErr != nil {
			slog.Error("parser: record source failure failed", "source", ds.ID, "err", recordErr)
		}
		if failures == 1 || failures%3 == 0 {
			s.enqueueAdminAlert(ctx, "parser-failure:"+logID, fmt.Sprintf(
				"⚠️ Ошибка обновления расписания\n\nИсточник: %s\nПопытка подряд: %d\nОшибка: %s",
				adapter.Name(), failures, truncate(message, 1200),
			))
		}
		return records, runErr
	}

	groups, err := adapter.FetchGroups(ctx)
	if err != nil {
		return fail(0, fmt.Errorf("parser: FetchGroups [%s]: %w", adapter.Name(), err))
	}
	if len(groups) == 0 {
		return fail(0, fmt.Errorf("parser: FetchGroups [%s] returned no groups", adapter.Name()))
	}

	results := s.fetchSchedules(ctx, adapter, groups)
	var fetchErrors []error
	for _, result := range results {
		if result.err != nil {
			fetchErrors = append(fetchErrors, fmt.Errorf("group %s: %w", result.group.Name, result.err))
		}
	}
	if len(fetchErrors) > 0 {
		return fail(0, fmt.Errorf(
			"parser: candidate was not published because %d/%d schedule requests failed: %w",
			len(fetchErrors), len(groups), errors.Join(fetchErrors...),
		))
	}

	payload, lessonCount := buildScheduleSnapshot(adapter.UniversityID(), semesterID, results)
	baseline, err := s.snapshotRepo.Baseline(ctx, adapter.UniversityID(), ds.ID)
	if err != nil {
		return fail(lessonCount, fmt.Errorf("parser: load baseline: %w", err))
	}
	anomalies, publishable := evaluateSnapshot(payload, lessonCount, baseline)
	status := domain.SnapshotStatusStaged
	if len(anomalies) > 0 {
		status = domain.SnapshotStatusQuarantined
	}
	snapshot := &domain.ParserSnapshot{
		ID:             uuid.NewString(),
		DataSourceID:   ds.ID,
		ParseLogID:     logID,
		Status:         status,
		Publishable:    publishable,
		GroupCount:     len(payload.Groups),
		LessonCount:    lessonCount,
		AnomalyReasons: anomalies,
		Payload:        payload,
	}
	if err = s.snapshotRepo.Create(ctx, snapshot); err != nil {
		return fail(lessonCount, fmt.Errorf("parser: stage snapshot: %w", err))
	}
	if status == domain.SnapshotStatusQuarantined {
		summary := anomalySummary(anomalies)
		_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "quarantined", lessonCount, summary)
		_ = s.dataSourceRepo.RecordQuarantine(ctx, ds.ID,
			fmt.Sprintf("Снимок %s помещён в карантин: %s", snapshot.ID, summary))
		s.enqueueAdminAlert(ctx, "parser-quarantine:"+snapshot.ID, fmt.Sprintf(
			"🛡 Снимок расписания помещён в карантин\n\nИсточник: %s\nГрупп: %d\nЗанятий: %d\nПричины: %s\n\nПроверьте снимок в админ-панели.",
			adapter.Name(), snapshot.GroupCount, snapshot.LessonCount, truncate(summary, 1800),
		))
		return lessonCount, fmt.Errorf("%w: snapshot=%s: %s", ErrSnapshotQuarantine, snapshot.ID, summary)
	}

	if _, err = s.publishSnapshot(ctx, snapshot.ID, "", "Автоматическая публикация"); err != nil {
		return fail(lessonCount, fmt.Errorf("parser: publish snapshot %s: %w", snapshot.ID, err))
	}
	_ = s.parseLogRepo.UpdateParseLog(ctx, logID, "success", lessonCount, "")
	_ = s.snapshotRepo.Prune(ctx, ds.ID, snapshot.ID)
	slog.Info("parser: data source run complete",
		"adapter", adapter.Name(),
		"groups", len(payload.Groups),
		"lessons", lessonCount,
		"snapshot", snapshot.ID,
		"elapsed", time.Since(startedAt),
	)
	return lessonCount, nil
}

func (s *ParserService) PublishSnapshot(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
) (*domain.ParserSnapshot, error) {
	candidate, err := s.snapshotRepo.Get(ctx, snapshotID)
	if err != nil || candidate == nil {
		return candidate, err
	}
	release, acquired, err := s.dataSourceRepo.TryAcquireRunLock(ctx, candidate.DataSourceID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, fmt.Errorf("%w: %s", ErrDataSourceBusy, candidate.DataSourceID)
	}
	defer func() { _ = release() }()
	return s.publishSnapshot(ctx, snapshotID, actorID, reviewNote)
}

func (s *ParserService) RejectSnapshot(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
) error {
	return s.snapshotRepo.Reject(ctx, snapshotID, actorID, reviewNote)
}

func (s *ParserService) RollbackSource(
	ctx context.Context,
	sourceID, actorID, reviewNote string,
) (*domain.ParserSnapshot, error) {
	release, acquired, err := s.dataSourceRepo.TryAcquireRunLock(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, fmt.Errorf("%w: %s", ErrDataSourceBusy, sourceID)
	}
	defer func() { _ = release() }()
	previous, err := s.snapshotRepo.PreviousPublished(ctx, sourceID)
	if err != nil || previous == nil {
		return previous, err
	}
	return s.publishSnapshot(ctx, previous.ID, actorID, reviewNote)
}

func (s *ParserService) publishSnapshot(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
) (*domain.ParserSnapshot, error) {
	candidate, err := s.snapshotRepo.Get(ctx, snapshotID)
	if err != nil || candidate == nil {
		return candidate, err
	}
	groupIDs := make(map[string]struct{}, len(candidate.Payload.Groups))
	for _, group := range candidate.Payload.Groups {
		groupIDs[group.ID] = struct{}{}
	}
	currentGroups, err := s.groupRepo.GetGroupsByUniversityID(ctx, candidate.Payload.UniversityID)
	if err != nil {
		return nil, err
	}
	for _, group := range currentGroups {
		groupIDs[group.ID] = struct{}{}
	}
	before, err := s.captureEffectiveSchedules(ctx, candidate.Payload.UniversityID, groupIDs)
	if err != nil {
		return nil, err
	}
	published, err := s.snapshotRepo.Publish(ctx, snapshotID, actorID, reviewNote)
	if err != nil {
		return nil, err
	}
	after, err := s.captureEffectiveSchedules(ctx, candidate.Payload.UniversityID, groupIDs)
	if err != nil {
		slog.Error("parser: published snapshot but failed to read resulting schedules",
			"snapshot", snapshotID, "err", err)
		return published, nil
	}
	for groupID := range groupIDs {
		diff := CompareLessonSnapshots(before[groupID], after[groupID])
		if !diff.Changed() || s.notificationRepo == nil {
			continue
		}
		if enqueueErr := s.notificationRepo.EnqueueScheduleChange(
			ctx, uuid.NewString(), groupID, "parser", scheduleChangeSummary(diff),
		); enqueueErr != nil {
			slog.Error("parser: enqueue schedule notification failed",
				"group", groupID, "snapshot", snapshotID, "err", enqueueErr)
		}
	}
	return published, nil
}

func (s *ParserService) captureEffectiveSchedules(
	ctx context.Context,
	universityID string,
	groupIDs map[string]struct{},
) (map[string][]domain.Lesson, error) {
	result := make(map[string][]domain.Lesson, len(groupIDs))
	for groupID := range groupIDs {
		result[groupID] = []domain.Lesson{}
	}
	lessons, err := s.scheduleSvc.GetAllLessonsForUniversity(ctx, universityID)
	if err != nil {
		return nil, fmt.Errorf("read effective schedule for university %s: %w", universityID, err)
	}
	for _, lesson := range lessons {
		result[lesson.GroupID] = append(result[lesson.GroupID], lesson)
	}
	return result, nil
}

func (s *ParserService) enqueueAdminAlert(ctx context.Context, id, body string) {
	if s.notificationRepo == nil {
		return
	}
	if err := s.notificationRepo.EnqueueAdminAlert(ctx, id, body); err != nil {
		slog.Error("parser: enqueue admin alert failed", "id", id, "err", err)
	}
}

func buildScheduleSnapshot(
	universityID, semesterID string,
	results []groupScheduleResult,
) (domain.ScheduleSnapshot, int) {
	payload := domain.ScheduleSnapshot{
		UniversityID: universityID,
		SemesterID:   semesterID,
		Groups:       make([]domain.SnapshotGroup, 0, len(results)),
	}
	total := 0
	for _, result := range results {
		group := domain.SnapshotGroup{
			ID:           result.group.ID,
			UniversityID: universityID,
			Name:         strings.TrimSpace(result.group.Name),
			Lessons:      result.lessons,
		}
		for i := range group.Lessons {
			lesson := &group.Lessons[i]
			lesson.UniversityID = universityID
			lesson.SemesterID = semesterID
			lesson.GroupID = group.ID
			if lesson.ValidFrom != nil && (payload.StartDate.IsZero() || lesson.ValidFrom.Before(payload.StartDate)) {
				payload.StartDate = *lesson.ValidFrom
			}
			if lesson.ValidTo != nil && (payload.EndDate.IsZero() || lesson.ValidTo.After(payload.EndDate)) {
				payload.EndDate = *lesson.ValidTo
			}
		}
		total += len(group.Lessons)
		payload.Groups = append(payload.Groups, group)
	}
	now := time.Now()
	if payload.StartDate.IsZero() {
		payload.StartDate = now
	}
	if payload.EndDate.IsZero() {
		payload.EndDate = payload.StartDate
	}
	return payload, total
}

func evaluateSnapshot(
	payload domain.ScheduleSnapshot,
	lessonCount int,
	baseline *domain.SnapshotBaseline,
) ([]domain.SnapshotAnomaly, bool) {
	anomalies := make([]domain.SnapshotAnomaly, 0)
	publishable := true
	groupIDs := make(map[string]struct{}, len(payload.Groups))
	groupNames := make(map[string]struct{}, len(payload.Groups))
	lessonIDs := make(map[string]struct{}, lessonCount)
	emptyPreviouslyPopulated := 0

	for _, group := range payload.Groups {
		switch {
		case strings.TrimSpace(group.ID) == "":
			anomalies = append(anomalies, structuralAnomaly("group_id_empty", "У группы отсутствует идентификатор"))
			publishable = false
		case group.UniversityID != payload.UniversityID:
			anomalies = append(anomalies, structuralAnomaly("group_university_mismatch", "Группа относится к другому учебному заведению"))
			publishable = false
		}
		if _, exists := groupIDs[group.ID]; exists {
			anomalies = append(anomalies, structuralAnomaly("group_id_duplicate", "В снимке повторяется идентификатор группы "+group.ID))
			publishable = false
		}
		groupIDs[group.ID] = struct{}{}
		normalizedName := strings.ToLower(strings.TrimSpace(group.Name))
		if normalizedName == "" {
			anomalies = append(anomalies, structuralAnomaly("group_name_empty", "У группы отсутствует название"))
			publishable = false
		} else if _, exists := groupNames[normalizedName]; exists {
			anomalies = append(anomalies, structuralAnomaly("group_name_duplicate", "В снимке повторяется название группы "+group.Name))
			publishable = false
		}
		groupNames[normalizedName] = struct{}{}
		if baseline.LessonsByGroup[group.ID] > 0 && len(group.Lessons) == 0 {
			emptyPreviouslyPopulated++
		}
		for _, lesson := range group.Lessons {
			if reason := validateSnapshotLesson(lesson, group.ID, payload); reason != "" {
				anomalies = append(anomalies, structuralAnomaly("lesson_invalid", reason))
				publishable = false
			}
			if _, exists := lessonIDs[lesson.ID]; exists {
				anomalies = append(anomalies, structuralAnomaly("lesson_id_duplicate", "Повторяется идентификатор занятия "+lesson.ID))
				publishable = false
			}
			lessonIDs[lesson.ID] = struct{}{}
		}
	}

	if lessonCount == 0 {
		anomalies = append(anomalies, domain.SnapshotAnomaly{
			Code: "all_lessons_empty", Message: "Источник вернул пустое расписание для всех групп",
		})
	}
	if !baseline.HasExistingState {
		return uniqueAnomalies(anomalies), publishable
	}
	groupRatio := safeRatio(len(payload.Groups), baseline.GroupCount)
	lessonRatio := safeRatio(lessonCount, baseline.LessonCount)
	if baseline.GroupCount >= 10 && groupRatio < 0.70 {
		anomalies = append(anomalies, ratioAnomaly(
			"group_count_drop", "Количество групп уменьшилось более чем на 30%",
			baseline.GroupCount, len(payload.Groups), groupRatio,
		))
	}
	if baseline.GroupCount >= 20 && groupRatio > 1.80 {
		anomalies = append(anomalies, ratioAnomaly(
			"group_count_spike", "Количество групп выросло более чем на 80%",
			baseline.GroupCount, len(payload.Groups), groupRatio,
		))
	}
	if baseline.LessonCount >= 20 && lessonRatio < 0.60 {
		anomalies = append(anomalies, ratioAnomaly(
			"lesson_count_drop", "Количество занятий уменьшилось более чем на 40%",
			baseline.LessonCount, lessonCount, lessonRatio,
		))
	}
	if baseline.LessonCount >= 20 && lessonRatio > 2.0 {
		anomalies = append(anomalies, ratioAnomaly(
			"lesson_count_spike", "Количество занятий выросло более чем в два раза",
			baseline.LessonCount, lessonCount, lessonRatio,
		))
	}
	emptyLimit := int(math.Max(3, math.Ceil(float64(baseline.GroupCount)*0.10)))
	if emptyPreviouslyPopulated >= emptyLimit {
		anomalies = append(anomalies, domain.SnapshotAnomaly{
			Code:      "many_groups_became_empty",
			Message:   fmt.Sprintf("У %d ранее заполненных групп исчезло всё расписание", emptyPreviouslyPopulated),
			Current:   baseline.GroupCount,
			Candidate: emptyPreviouslyPopulated,
		})
	}
	return uniqueAnomalies(anomalies), publishable
}

func validateSnapshotLesson(
	lesson domain.Lesson,
	groupID string,
	payload domain.ScheduleSnapshot,
) string {
	switch {
	case strings.TrimSpace(lesson.ID) == "":
		return "У занятия отсутствует идентификатор"
	case lesson.GroupID != groupID:
		return "Занятие " + lesson.ID + " привязано к другой группе"
	case lesson.UniversityID != payload.UniversityID:
		return "Занятие " + lesson.ID + " относится к другому учебному заведению"
	case strings.TrimSpace(lesson.Subject) == "":
		return "У занятия " + lesson.ID + " отсутствует дисциплина"
	case strings.TrimSpace(lesson.TimeStart) == "" || strings.TrimSpace(lesson.TimeEnd) == "":
		return "У занятия " + lesson.ID + " не указано время"
	case lesson.ValidFrom != nil && lesson.ValidTo != nil && lesson.ValidFrom.After(*lesson.ValidTo):
		return "У занятия " + lesson.ID + " некорректный период действия"
	}
	switch lesson.WeekType {
	case domain.WeekTypeDate:
		if lesson.SpecialDate == nil {
			return "У разового занятия " + lesson.ID + " отсутствует дата"
		}
	case domain.WeekTypeEvery, domain.WeekTypeOdd, domain.WeekTypeEven:
		if lesson.DayOfWeek < 1 || lesson.DayOfWeek > 7 {
			return "У занятия " + lesson.ID + " некорректный день недели"
		}
	default:
		return "У занятия " + lesson.ID + " неизвестный тип недели"
	}
	return ""
}

func structuralAnomaly(code, message string) domain.SnapshotAnomaly {
	return domain.SnapshotAnomaly{Code: code, Message: message}
}

func ratioAnomaly(code, message string, current, candidate int, ratio float64) domain.SnapshotAnomaly {
	return domain.SnapshotAnomaly{
		Code: code, Message: message, Current: current, Candidate: candidate, Ratio: ratio,
	}
}

func safeRatio(candidate, current int) float64 {
	if current == 0 {
		if candidate == 0 {
			return 1
		}
		return math.Inf(1)
	}
	return float64(candidate) / float64(current)
}

func uniqueAnomalies(items []domain.SnapshotAnomaly) []domain.SnapshotAnomaly {
	seen := make(map[string]bool, len(items))
	result := make([]domain.SnapshotAnomaly, 0, len(items))
	for _, item := range items {
		key := item.Code + "\x00" + item.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func anomalySummary(items []domain.SnapshotAnomaly) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Message)
	}
	return strings.Join(parts, "; ")
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

func (s *ParserService) fetchSchedules(
	ctx context.Context,
	adapter scrapper.SourceAdapter,
	groups []domain.Group,
) []groupScheduleResult {
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
			lessons, err := fetchScheduleWithRetry(ctx, adapter, group.ID)
			results[i] = groupScheduleResult{group: group, lessons: lessons, err: err}
		}()
	}
	wg.Wait()
	return results
}

func fetchScheduleWithRetry(
	ctx context.Context,
	adapter scrapper.SourceAdapter,
	groupID string,
) ([]domain.Lesson, error) {
	var lastErr error
	for attempt := 1; attempt <= scheduleFetchAttempts; attempt++ {
		lessons, err := adapter.FetchSchedule(ctx, groupID)
		if err == nil {
			return lessons, nil
		}
		lastErr = err
		if attempt == scheduleFetchAttempts || ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
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

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "…"
}
