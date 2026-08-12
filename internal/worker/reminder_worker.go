package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

const (
	reminderLookaheadDays = 1
	reminderBatchSize     = 250
	reminderRunTimeout    = 45 * time.Second
	workerStatusTimeout   = 3 * time.Second
)

type reminderRecipientRepository interface {
	ActiveRecipientsPage(context.Context, string, int) ([]domain.ReminderRecipient, error)
	Enqueue(context.Context, string, string, string, string) error
}

type reminderScheduleProvider interface {
	GetScheduleForGroup(context.Context, string, time.Time) ([]domain.Lesson, error)
}

type workerStatusRepository interface {
	Get(context.Context, string) (*domain.WorkerStatus, error)
	RecordRun(context.Context, domain.WorkerRunResult) error
}

type ReminderWorker struct {
	repository       reminderRecipientRepository
	statusRepository workerStatusRepository
	scheduleService  reminderScheduleProvider
	interval         time.Duration
	runTimeout       time.Duration
	now              func() time.Time
	locations        map[string]*time.Location
}

type reminderSlot struct {
	TimeStart string
	TimeEnd   string
	Lessons   []domain.Lesson
}

type reminderScheduleCache map[string]map[string][]domain.Lesson

func NewReminderWorker(
	repository *repository.ReminderRepository,
	statusRepository *repository.WorkerStatusRepository,
	scheduleService *service.ScheduleService,
	interval time.Duration,
) *ReminderWorker {
	return &ReminderWorker{
		repository:       repository,
		statusRepository: statusRepository,
		scheduleService:  scheduleService,
		interval:         interval,
		runTimeout:       reminderRunTimeout,
		now:              time.Now,
		locations:        make(map[string]*time.Location),
	}
}

func (w *ReminderWorker) Start(ctx context.Context, monitors ...*Monitor) <-chan struct{} {
	done := make(chan struct{})
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	go func() {
		defer close(done)
		monitor.Started(ReminderWorkerName)
		defer monitor.Stopped(ReminderWorkerName)
		w.run(ctx, monitor)
	}()
	return done
}

func (w *ReminderWorker) run(ctx context.Context, monitor *Monitor) {
	slog.Info("lesson reminder worker started", "interval", w.interval)
	monitor.Record(ReminderWorkerName, w.tick(ctx))
	monitor.Heartbeat(ReminderWorkerName)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("lesson reminder worker stopped")
			return
		case <-ticker.C:
			monitor.Record(ReminderWorkerName, w.tick(ctx))
			monitor.Heartbeat(ReminderWorkerName)
		}
	}
}

func (w *ReminderWorker) tick(parent context.Context) error {
	startedAt := w.now()
	ctx, cancel := context.WithTimeout(parent, w.runTimeout)
	defer cancel()

	status, err := w.statusRepository.Get(ctx, domain.LessonReminderWorker)
	if err != nil {
		slog.Error("lesson reminder worker: load cursor failed", "err", err)
		return err
	}

	schedules := make(reminderScheduleCache)
	afterUserID := status.Cursor
	processed := 0
	failures := 0
	fullCycle := false
	var runErr error

scan:
	for {
		recipients, pageErr := w.repository.ActiveRecipientsPage(
			ctx,
			afterUserID,
			reminderBatchSize,
		)
		if pageErr != nil {
			runErr = fmt.Errorf("load recipients after %q: %w", afterUserID, pageErr)
			break
		}
		if len(recipients) == 0 {
			afterUserID = ""
			fullCycle = true
			break
		}
		for _, recipient := range recipients {
			if ctx.Err() != nil {
				runErr = ctx.Err()
				break scan
			}
			recipientErr := w.enqueueRecipientReminders(ctx, recipient, startedAt, schedules)
			if ctx.Err() != nil {
				runErr = ctx.Err()
				break scan
			}
			processed++
			afterUserID = recipient.UserID
			if recipientErr != nil {
				failures++
				if runErr == nil {
					runErr = recipientErr
				}
			}
		}
		if len(recipients) < reminderBatchSize {
			afterUserID = ""
			fullCycle = true
			break
		}
	}

	if runErr != nil && failures == 0 {
		failures = 1
	}
	finishedAt := w.now()
	recordErr := w.recordRun(parent, domain.WorkerRunResult{
		Name:            domain.LessonReminderWorker,
		Cursor:          afterUserID,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		LastFullCycleAt: fullCycleTimestamp(fullCycle, finishedAt),
		Processed:       processed,
		Failures:        failures,
		LastError:       compactWorkerError(runErr),
	})
	if runErr != nil {
		return runErr
	}
	return recordErr
}

func (w *ReminderWorker) recordRun(parent context.Context, result domain.WorkerRunResult) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), workerStatusTimeout)
	defer cancel()
	if err := w.statusRepository.RecordRun(ctx, result); err != nil {
		slog.Error("lesson reminder worker: record run failed", "err", err)
		return err
	}
	return nil
}

func fullCycleTimestamp(completed bool, finishedAt time.Time) *time.Time {
	if !completed {
		return nil
	}
	return &finishedAt
}

func compactWorkerError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func (w *ReminderWorker) enqueueRecipientReminders(
	ctx context.Context,
	recipient domain.ReminderRecipient,
	now time.Time,
	schedules reminderScheduleCache,
) error {
	location, err := w.recipientLocation(recipient.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %q for group %s: %w", recipient.Timezone, recipient.GroupID, err)
	}
	now = now.In(location)
	var firstErr error
	for offset := 0; offset <= reminderLookaheadDays; offset++ {
		date := dateOnly(now).AddDate(0, 0, offset)
		dateKey := date.Format("2006-01-02")
		groupSchedules := schedules[recipient.GroupID]
		if groupSchedules == nil {
			groupSchedules = make(map[string][]domain.Lesson)
			schedules[recipient.GroupID] = groupSchedules
		}

		lessons, loaded := groupSchedules[dateKey]
		if !loaded {
			var err error
			lessons, err = w.scheduleService.GetScheduleForGroup(ctx, recipient.GroupID, date)
			if err != nil {
				slog.Error(
					"lesson reminder worker: load schedule failed",
					"group_id", recipient.GroupID,
					"date", dateKey,
					"err", err,
				)
				if firstErr == nil {
					firstErr = err
				}
				groupSchedules[dateKey] = []domain.Lesson{}
				continue
			}
			groupSchedules[dateKey] = lessons
		}

		for _, slot := range reminderSlots(lessons) {
			if err := w.enqueueReminderSlot(ctx, recipient, date, now, slot); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *ReminderWorker) recipientLocation(name string) (*time.Location, error) {
	if location := w.locations[name]; location != nil {
		return location, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	if w.locations == nil {
		w.locations = make(map[string]*time.Location)
	}
	w.locations[name] = location
	return location, nil
}

func (w *ReminderWorker) enqueueReminderSlot(
	ctx context.Context,
	recipient domain.ReminderRecipient,
	date time.Time,
	now time.Time,
	slot reminderSlot,
) error {
	startsAt, err := lessonStart(date, slot.TimeStart)
	if err != nil {
		slog.Warn(
			"lesson reminder worker: invalid lesson time",
			"group_id", recipient.GroupID,
			"time_start", slot.TimeStart,
			"err", err,
		)
		return err
	}
	until := startsAt.Sub(now)
	if until <= 0 || until > time.Duration(recipient.ReminderMinutes)*time.Minute {
		return nil
	}

	id := reminderID(
		recipient.UserID,
		recipient.GroupID,
		date,
		slot.TimeStart,
		slot.TimeEnd,
	)
	body := reminderText(recipient, date, slot, until)
	if err := w.repository.Enqueue(
		ctx,
		id,
		recipient.UserID,
		recipient.GroupID,
		body,
	); err != nil {
		slog.Error(
			"lesson reminder worker: enqueue failed",
			"user_id", recipient.UserID,
			"group_id", recipient.GroupID,
			"err", err,
		)
		return err
	}
	return nil
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func lessonStart(date time.Time, value string) (time.Time, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		parsed, err := time.ParseInLocation(layout, value, date.Location())
		if err == nil {
			year, month, day := date.Date()
			return time.Date(
				year,
				month,
				day,
				parsed.Hour(),
				parsed.Minute(),
				parsed.Second(),
				0,
				date.Location(),
			), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time %q", value)
}

func reminderSlots(lessons []domain.Lesson) []reminderSlot {
	byTime := make(map[string]*reminderSlot)
	order := make([]string, 0)
	for _, lesson := range lessons {
		key := lesson.TimeStart + "|" + lesson.TimeEnd
		slot := byTime[key]
		if slot == nil {
			slot = &reminderSlot{
				TimeStart: lesson.TimeStart,
				TimeEnd:   lesson.TimeEnd,
			}
			byTime[key] = slot
			order = append(order, key)
		}
		slot.Lessons = append(slot.Lessons, lesson)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return byTime[order[i]].TimeStart < byTime[order[j]].TimeStart
	})
	result := make([]reminderSlot, 0, len(order))
	for _, key := range order {
		result = append(result, *byTime[key])
	}
	return result
}

func reminderID(
	userID string,
	groupID string,
	date time.Time,
	timeStart string,
	timeEnd string,
) string {
	sum := sha256.Sum256([]byte(strings.Join(
		[]string{userID, groupID, date.Format("2006-01-02"), timeStart, timeEnd},
		"|",
	)))
	return "lesson-reminder:" + hex.EncodeToString(sum[:16])
}

func reminderText(
	recipient domain.ReminderRecipient,
	date time.Time,
	slot reminderSlot,
	until time.Duration,
) string {
	minutes := max(1, int(math.Ceil(until.Minutes())))
	var text strings.Builder
	fmt.Fprintf(
		&text,
		"⏰ Пара начнётся через %d мин.\n\n%s · %s\n%s, %s–%s\n",
		minutes,
		recipient.UniversityName,
		recipient.GroupName,
		date.Format("02.01.2006"),
		slot.TimeStart,
		slot.TimeEnd,
	)
	for _, lesson := range slot.Lessons {
		fmt.Fprintf(&text, "\n• %s", lesson.Subject)
		if lesson.Subgroup > 0 {
			fmt.Fprintf(&text, ", подгруппа %d", lesson.Subgroup)
		}
		if lesson.Room != "" {
			text.WriteString(" · ")
			text.WriteString(lesson.Room)
		}
		if lesson.Teacher != "" {
			text.WriteString("\n  ")
			text.WriteString(lesson.Teacher)
		}
	}
	text.WriteString("\n\nНастроить напоминания: /reminders")
	return text.String()
}
