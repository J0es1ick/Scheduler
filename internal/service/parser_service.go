package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scraper"
	"github.com/google/uuid"
)

const scheduleFetchConcurrency = 3
const scheduleFetchAttempts = 3
const scheduleCircuitThreshold = 3
const parserDiagnosticRetention = 30 * 24 * time.Hour

var (
	ErrDataSourceBusy     = errors.New("parser: data source is already running")
	ErrDataSourceDisabled = errors.New("parser: data source is disabled")
	ErrSnapshotQuarantine = errors.New("parser: candidate snapshot quarantined")
)

type ParserService struct {
	dataSourceRepo   *repository.DataSourceRepository
	parseLogRepo     *repository.ParseLogRepository
	groupRepo        *repository.GroupRepository
	scheduleSvc      *ScheduleService
	snapshotRepo     *repository.ParserSnapshotRepository
	notificationRepo *repository.NotificationRepository
	diagnosticRepo   *repository.ParserDiagnosticRepository
	adapters         map[string]scraper.SourceAdapter
}

func NewParserService(
	dataSourceRepo *repository.DataSourceRepository,
	parseLogRepo *repository.ParseLogRepository,
	groupRepo *repository.GroupRepository,
	scheduleSvc *ScheduleService,
	snapshotRepo *repository.ParserSnapshotRepository,
	notificationRepo *repository.NotificationRepository,
	diagnosticRepo *repository.ParserDiagnosticRepository,
) *ParserService {
	return &ParserService{
		dataSourceRepo:   dataSourceRepo,
		parseLogRepo:     parseLogRepo,
		groupRepo:        groupRepo,
		scheduleSvc:      scheduleSvc,
		snapshotRepo:     snapshotRepo,
		notificationRepo: notificationRepo,
		diagnosticRepo:   diagnosticRepo,
		adapters:         make(map[string]scraper.SourceAdapter),
	}
}

func (s *ParserService) RegisterAdapter(adapterType string, adapter scraper.SourceAdapter) {
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
	if !ds.IsEnabled {
		return 0, fmt.Errorf("%w: %s", ErrDataSourceDisabled, dataSourceID)
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
		failures, nextRetryAt, recordErr := s.dataSourceRepo.RecordFailure(
			ctx,
			ds.ID,
			message,
		)
		if recordErr != nil {
			slog.Error("parser: record source failure failed", "source", ds.ID, "err", recordErr)
		}
		if failures == 1 || failures%3 == 0 {
			s.enqueueAdminAlert(ctx, "parser-failure:"+logID, fmt.Sprintf(
				"⚠️ Ошибка обновления расписания\n\nИсточник: %s\n"+
					"Попытка подряд: %d\nСледующая попытка: %s\nОшибка: %s",
				adapter.Name(),
				failures,
				nextRetryAt.In(time.Local).Format("02.01.2006 15:04"),
				truncate(message, 1200),
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

	fetchReport := s.fetchSchedules(ctx, adapter, groups)
	s.saveScheduleDiagnostics(ctx, logID, ds.ID, fetchReport)
	if fetchReport.Failed > 0 {
		return fail(0, fetchReport.Error(len(groups)))
	}
	results := fetchReport.Results

	payload, lessonCount := buildScheduleSnapshot(adapter.UniversityID(), semesterID, results)
	existingGroups, err := s.groupRepo.GetGroupsByUniversityID(ctx, adapter.UniversityID())
	if err != nil {
		return fail(lessonCount, fmt.Errorf("parser: load existing group identities: %w", err))
	}
	payload, remappedGroups, err := repository.CanonicalizeSnapshotGroupIDs(payload, existingGroups)
	if err != nil {
		return fail(lessonCount, fmt.Errorf("parser: reconcile group identities: %w", err))
	}
	if remappedGroups > 0 {
		slog.Info(
			"parser: source group identifiers reconciled",
			"adapter", adapter.Name(),
			"groups", remappedGroups,
		)
	}
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
	var hook repository.SnapshotPublicationHook
	if s.notificationRepo != nil {
		hook = func(ctx context.Context, publication *repository.SnapshotPublication) error {
			afterLessons, err := publication.EffectiveLessonsByUniversity(
				ctx, candidate.Payload.UniversityID,
			)
			if err != nil {
				return err
			}
			after := schedulesByGroup(groupIDs, afterLessons)
			for groupID := range groupIDs {
				diff := CompareLessonSnapshots(before[groupID], after[groupID])
				if !diff.Changed() {
					continue
				}
				if err = publication.EnqueueScheduleChange(
					ctx, uuid.NewString(), groupID, "parser", scheduleChangeSummary(diff),
				); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return s.snapshotRepo.PublishWithHook(ctx, snapshotID, actorID, reviewNote, hook)
}

func (s *ParserService) captureEffectiveSchedules(
	ctx context.Context,
	universityID string,
	groupIDs map[string]struct{},
) (map[string][]domain.Lesson, error) {
	lessons, err := s.scheduleSvc.GetAllLessonsForUniversity(ctx, universityID)
	if err != nil {
		return nil, fmt.Errorf("read effective schedule for university %s: %w", universityID, err)
	}
	return schedulesByGroup(groupIDs, lessons), nil
}

func schedulesByGroup(
	groupIDs map[string]struct{},
	lessons []domain.Lesson,
) map[string][]domain.Lesson {
	result := make(map[string][]domain.Lesson, len(groupIDs))
	for groupID := range groupIDs {
		result[groupID] = []domain.Lesson{}
	}
	for _, lesson := range lessons {
		result[lesson.GroupID] = append(result[lesson.GroupID], lesson)
	}
	return result
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

	if baseline.TrustedSnapshot != nil &&
		scheduleSnapshotsEquivalent(payload, *baseline.TrustedSnapshot) {
		return uniqueAnomalies(anomalies), publishable
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

type scheduleDiagnosticAggregate struct {
	Diagnostic   scraper.ResponseDiagnostic
	FirstGroupID string
	Occurrences  int
}

type scheduleFetchReport struct {
	Results     []groupScheduleResult
	Attempted   int
	Failed      int
	Skipped     int
	FirstError  error
	Diagnostics map[string]*scheduleDiagnosticAggregate
	Circuit     *scheduleDiagnosticAggregate
}

func (r scheduleFetchReport) Error(total int) error {
	if r.Circuit != nil {
		return fmt.Errorf(
			"parser: обновление не опубликовано: %s; "+
				"одинаковый ответ получен %d раза, проверено %d из %d групп, "+
				"пропущено %d; рабочее расписание сохранено",
			r.Circuit.Diagnostic.Summary,
			r.Circuit.Occurrences,
			r.Attempted,
			total,
			r.Skipped,
		)
	}
	first := "неизвестная ошибка"
	if r.FirstError != nil {
		first = truncate(r.FirstError.Error(), 500)
	}
	return fmt.Errorf(
		"parser: обновление не опубликовано: ошибки у %d из %d проверенных групп; "+
			"первая ошибка: %s; рабочее расписание сохранено",
		r.Failed,
		r.Attempted,
		first,
	)
}

func (s *ParserService) fetchSchedules(
	ctx context.Context,
	adapter scraper.SourceAdapter,
	groups []domain.Group,
) scheduleFetchReport {
	report := scheduleFetchReport{
		Results:     make([]groupScheduleResult, len(groups)),
		Diagnostics: make(map[string]*scheduleDiagnosticAggregate),
	}
	for batchStart := 0; batchStart < len(groups); batchStart += scheduleFetchConcurrency {
		batchEnd := min(batchStart+scheduleFetchConcurrency, len(groups))
		var wg sync.WaitGroup
		for index := batchStart; index < batchEnd; index++ {
			index := index
			group := groups[index]
			wg.Add(1)
			go func() {
				defer wg.Done()
				lessons, err := fetchScheduleWithRetry(ctx, adapter, group.ID)
				report.Results[index] = groupScheduleResult{
					group:   group,
					lessons: lessons,
					err:     err,
				}
			}()
		}
		wg.Wait()
		report.Attempted += batchEnd - batchStart

		for index := batchStart; index < batchEnd; index++ {
			result := report.Results[index]
			if result.err == nil {
				continue
			}
			report.Failed++
			if report.FirstError == nil {
				report.FirstError = fmt.Errorf(
					"group %s: %w",
					result.group.Name,
					result.err,
				)
			}
			diagnostic, ok := scraper.ExtractResponseDiagnostic(result.err)
			if !ok {
				continue
			}
			key := diagnosticKey(diagnostic)
			aggregate := report.Diagnostics[key]
			if aggregate == nil {
				aggregate = &scheduleDiagnosticAggregate{
					Diagnostic:   diagnostic,
					FirstGroupID: result.group.ID,
				}
				report.Diagnostics[key] = aggregate
			}
			aggregate.Occurrences++
			if diagnostic.StopBatch &&
				aggregate.Occurrences >= scheduleCircuitThreshold &&
				report.Circuit == nil {
				report.Circuit = aggregate
			}
		}
		if report.Circuit != nil || ctx.Err() != nil {
			break
		}
	}
	report.Skipped = len(groups) - report.Attempted
	return report
}

func fetchScheduleWithRetry(
	ctx context.Context,
	adapter scraper.SourceAdapter,
	groupID string,
) ([]domain.Lesson, error) {
	var lastErr error
	for attempt := 1; attempt <= scheduleFetchAttempts; attempt++ {
		lessons, err := adapter.FetchSchedule(ctx, groupID)
		if err == nil {
			return lessons, nil
		}
		lastErr = err
		if diagnostic, ok := scraper.ExtractResponseDiagnostic(err); ok &&
			!diagnostic.Retryable {
			break
		}
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

func diagnosticKey(diagnostic scraper.ResponseDiagnostic) string {
	return strings.Join(
		[]string{
			diagnostic.Category,
			fmt.Sprint(diagnostic.HTTPStatus),
			diagnostic.ResponseSHA256,
		},
		"|",
	)
}

func (s *ParserService) saveScheduleDiagnostics(
	ctx context.Context,
	logID string,
	dataSourceID string,
	report scheduleFetchReport,
) {
	if s.diagnosticRepo == nil || len(report.Diagnostics) == 0 {
		return
	}
	keys := make([]string, 0, len(report.Diagnostics))
	for key := range report.Diagnostics {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 10 {
		keys = keys[:10]
	}
	for _, key := range keys {
		aggregate := report.Diagnostics[key]
		metadata, _ := json.Marshal(map[string]any{
			"attempted_groups": report.Attempted,
			"failed_groups":    report.Failed,
			"skipped_groups":   report.Skipped,
			"circuit_open":     report.Circuit == aggregate,
		})
		diagnostic := aggregate.Diagnostic
		item := &domain.ParserDiagnostic{
			ID:              uuid.NewString(),
			ParseLogID:      logID,
			DataSourceID:    dataSourceID,
			Stage:           "schedule_fetch",
			Category:        diagnostic.Category,
			Summary:         diagnostic.Summary,
			GroupID:         aggregate.FirstGroupID,
			HTTPStatus:      diagnostic.HTTPStatus,
			ContentType:     diagnostic.ContentType,
			ResponseSize:    diagnostic.ResponseSize,
			ResponseSHA256:  diagnostic.ResponseSHA256,
			ResponsePreview: diagnostic.ResponsePreview,
			Occurrences:     aggregate.Occurrences,
			Metadata:        metadata,
		}
		if err := s.diagnosticRepo.Create(ctx, item); err != nil {
			slog.Error(
				"parser: save response diagnostic failed",
				"source", dataSourceID,
				"category", diagnostic.Category,
				"err", err,
			)
		}
	}
	if err := s.diagnosticRepo.Prune(ctx, parserDiagnosticRetention); err != nil {
		slog.Error("parser: prune response diagnostics failed", "err", err)
	}
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
