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

const reminderLookaheadDays = 1

type ReminderWorker struct {
	repository      *repository.ReminderRepository
	scheduleService *service.ScheduleService
	interval        time.Duration
	now             func() time.Time
}

type reminderSlot struct {
	TimeStart string
	TimeEnd   string
	Lessons   []domain.Lesson
}

func NewReminderWorker(
	repository *repository.ReminderRepository,
	scheduleService *service.ScheduleService,
	interval time.Duration,
) *ReminderWorker {
	return &ReminderWorker{
		repository:      repository,
		scheduleService: scheduleService,
		interval:        interval,
		now:             time.Now,
	}
}

func (w *ReminderWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *ReminderWorker) run(ctx context.Context) {
	slog.Info("lesson reminder worker started", "interval", w.interval)
	w.tick(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("lesson reminder worker stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *ReminderWorker) tick(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	recipients, err := w.repository.ActiveRecipients(ctx)
	if err != nil {
		slog.Error("lesson reminder worker: load recipients failed", "err", err)
		return
	}
	now := w.now().In(time.Local)
	schedules := make(map[string]map[string][]domain.Lesson)
	for _, recipient := range recipients {
		if ctx.Err() != nil {
			return
		}
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
				lessons, err = w.scheduleService.GetScheduleForGroup(
					ctx,
					recipient.GroupID,
					date,
				)
				if err != nil {
					slog.Error(
						"lesson reminder worker: load schedule failed",
						"group_id", recipient.GroupID,
						"date", dateKey,
						"err", err,
					)
					groupSchedules[dateKey] = []domain.Lesson{}
					continue
				}
				groupSchedules[dateKey] = lessons
			}
			for _, slot := range reminderSlots(lessons) {
				startsAt, parseErr := lessonStart(date, slot.TimeStart)
				if parseErr != nil {
					slog.Warn(
						"lesson reminder worker: invalid lesson time",
						"group_id", recipient.GroupID,
						"time_start", slot.TimeStart,
						"err", parseErr,
					)
					continue
				}
				until := startsAt.Sub(now)
				if until <= 0 ||
					until > time.Duration(recipient.ReminderMinutes)*time.Minute {
					continue
				}
				id := reminderID(
					recipient.UserID,
					recipient.GroupID,
					date,
					slot.TimeStart,
					slot.TimeEnd,
				)
				body := reminderText(recipient, date, slot, until)
				if err = w.repository.Enqueue(
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
				}
			}
		}
	}
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
