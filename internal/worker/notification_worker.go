package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

const notificationBatchSize = 250
const notificationRetention = 90 * 24 * time.Hour

var errTelegramRateLimited = errors.New("telegram delivery rate limited")

type notificationDeliveryRepository interface {
	ClaimPending(context.Context, int) ([]domain.NotificationDelivery, error)
	ClaimBotOutbox(context.Context, int) ([]domain.BotOutboxDelivery, error)
	RenewDeliveryClaims(context.Context, string, []string) error
	RenewBotOutboxClaims(context.Context, string, []string) error
	DeliveryDecision(context.Context, string, string) (repository.NotificationQueueDecision, error)
	BotOutboxDecision(context.Context, string, string) (repository.NotificationQueueDecision, error)
	MarkDelivered(context.Context, string, string) error
	MarkCancelled(context.Context, string, string) error
	MarkDeferred(context.Context, string, string) error
	MarkFailed(context.Context, string, string, int, time.Duration, error) error
	MarkRateLimited(context.Context, string, string, time.Duration, error) error
	MarkPermanentFailure(context.Context, string, string, error) error
	MarkBotOutboxDelivered(context.Context, string, string) error
	MarkBotOutboxCancelled(context.Context, string, string) error
	MarkBotOutboxDeferred(context.Context, string, string) error
	MarkBotOutboxFailed(context.Context, string, string, int, time.Duration, error) error
	MarkBotOutboxRateLimited(context.Context, string, string, time.Duration, error) error
	MarkBotOutboxPermanentFailure(context.Context, string, string, error) error
	PruneCompleted(context.Context, time.Duration) (int64, error)
}

type notificationSender interface {
	Send(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error)
}

type NotificationWorker struct {
	reminderSchedule   reminderScheduleProvider
	reminderRecipient  func(context.Context, string, string) (*domain.ReminderRecipient, error)
	backgroundLimiter  *telegramlimit.Limiter
	batchSize          int
	maxBatches         int
	waitSend           func(context.Context, string) error
	repository         notificationDeliveryRepository
	bot                notificationSender
	interval           time.Duration
	claimRenewInterval time.Duration
	limiter            *telegramlimit.Limiter
}

type NotificationOptions struct {
	Schedule   reminderScheduleProvider
	BatchSize  int
	MaxBatches int
	Limiter    *telegramlimit.Limiter
}

func NewNotificationWorker(
	repository *repository.NotificationRepository,
	bot *tele.Bot,
	interval time.Duration,
	options ...NotificationOptions,
) *NotificationWorker {
	settings := NotificationOptions{BatchSize: notificationBatchSize, MaxBatches: 4}
	if len(options) > 0 {
		settings = options[0]
	}
	if interval <= 0 {
		interval = time.Second
	}
	limiter := settings.Limiter
	if limiter == nil {
		limiter = telegramlimit.New(
			telegramlimit.DefaultGlobalInterval,
			telegramlimit.DefaultRecipientInterval,
		)
	}
	return &NotificationWorker{
		reminderSchedule: settings.Schedule, reminderRecipient: repository.ReminderRecipient,
		backgroundLimiter:  telegramlimit.New(time.Second/15, 0),
		batchSize:          min(1000, max(1, settings.BatchSize)),
		maxBatches:         min(20, max(1, settings.MaxBatches)),
		repository:         repository,
		bot:                bot,
		interval:           interval,
		claimRenewInterval: notificationClaimRenewInterval,
		limiter:            limiter,
	}
}

func (w *NotificationWorker) Start(ctx context.Context, monitors ...*Monitor) <-chan struct{} {
	done := make(chan struct{})
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	go func() {
		defer close(done)
		monitor.Started(NotificationWorkerName)
		defer monitor.Stopped(NotificationWorkerName)
		w.run(ctx, monitor)
	}()
	return done
}

func (w *NotificationWorker) run(ctx context.Context, monitor *Monitor) {
	slog.Info("notification worker started", "interval", w.interval)
	w.prune(ctx)
	monitor.Record(NotificationWorkerName, w.tick(ctx, monitor))
	monitor.Heartbeat(NotificationWorkerName)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	pruneTicker := time.NewTicker(24 * time.Hour)
	defer pruneTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("notification worker stopped")
			return
		case <-ticker.C:
			monitor.Record(NotificationWorkerName, w.tick(ctx, monitor))
			monitor.Heartbeat(NotificationWorkerName)
		case <-pruneTicker.C:
			w.prune(ctx)
			monitor.Heartbeat(NotificationWorkerName)
		}
	}
}

func (w *NotificationWorker) prune(ctx context.Context) {
	deleted, err := w.repository.PruneCompleted(ctx, notificationRetention)
	if err != nil {
		slog.Error("notification worker: cleanup failed", "err", err)
		return
	}
	if deleted > 0 {
		slog.Info("notification history pruned", "events", deleted)
	}
}

func (w *NotificationWorker) tick(ctx context.Context, monitors ...*Monitor) error {
	var monitor *Monitor
	if len(monitors) > 0 {
		monitor = monitors[0]
	}
	batchSize := w.batchSize
	if batchSize <= 0 {
		batchSize = notificationBatchSize
	}
	maxBatches := w.maxBatches
	if maxBatches <= 0 {
		maxBatches = 4
	}
	started := time.Now()
	for range maxBatches {
		if err := ctx.Err(); err != nil {
			return err
		}
		full, err := w.drainBatch(ctx, monitor, batchSize)
		if err != nil {
			return err
		}
		monitor.Heartbeat(NotificationWorkerName)
		if !full || time.Since(started) >= 20*time.Second {
			break
		}
	}
	return nil
}

func (w *NotificationWorker) drainBatch(ctx context.Context, monitor *Monitor, batchSize int) (bool, error) {
	scheduleLimit := min(batchSize, 30)
	items, err := w.repository.ClaimPending(ctx, scheduleLimit)
	if err != nil {
		slog.Error("notification worker: claim failed", "err", err)
		return false, err
	}
	if len(items) > 0 {
		ids := make([]string, len(items))
		for index, item := range items {
			ids[index] = item.ID
		}
		if err = w.withClaims(ctx, items[0].ClaimToken, ids, w.repository.RenewDeliveryClaims,
			monitor, func(guard *notificationClaimGuard) error {
				return w.deliverScheduleBatch(guard.ctx, items, guard)
			}); err != nil {
			if errors.Is(err, errTelegramRateLimited) {
				return false, nil
			}
			return false, err
		}
	}

	outbox, err := w.repository.ClaimBotOutbox(ctx, batchSize)
	if err != nil {
		slog.Error("notification worker: claim bot outbox failed", "err", err)
		return false, err
	}
	if len(outbox) == 0 {
		return len(items) >= scheduleLimit, nil
	}
	ids := make([]string, len(outbox))
	for index, item := range outbox {
		ids[index] = item.ID
	}
	err = w.withClaims(ctx, outbox[0].ClaimToken, ids, w.repository.RenewBotOutboxClaims,
		monitor, func(guard *notificationClaimGuard) error {
			for index, item := range outbox {
				deliveryErr := w.deliverBotOutbox(guard.ctx, item, guard)
				if _, limited := telegramlimit.FloodRetryAfter(deliveryErr); limited {
					for _, pending := range outbox[index+1:] {
						if markErr := guard.finish(pending.ID, func(markCtx context.Context) error {
							return w.recordBotOutboxFailure(markCtx, pending, deliveryErr)
						}); markErr != nil {
							return markErr
						}
					}
					return errTelegramRateLimited
				}
				if deliveryErr != nil {
					return deliveryErr
				}
			}
			return nil
		})
	if errors.Is(err, errTelegramRateLimited) {
		return false, nil
	}
	return len(items) >= scheduleLimit || len(outbox) >= batchSize, err
}

func (w *NotificationWorker) withClaims(
	ctx context.Context,
	token string,
	ids []string,
	renew func(context.Context, string, []string) error,
	monitor *Monitor,
	deliver func(*notificationClaimGuard) error,
) error {
	renewInterval := w.claimRenewInterval
	if renewInterval <= 0 {
		renewInterval = notificationClaimRenewInterval
	}
	guard, err := startNotificationClaimGuard(ctx, token, ids, renew,
		func() { monitor.Heartbeat(NotificationWorkerName) }, renewInterval)
	if err != nil {
		return err
	}
	defer guard.stop()
	if err = deliver(guard); err != nil {
		return err
	}
	return context.Cause(guard.ctx)
}

func (w *NotificationWorker) deliverScheduleBatch(
	ctx context.Context,
	items []domain.NotificationDelivery,
	guard *notificationClaimGuard,
) error {
	byUser := make(map[string][]domain.NotificationDelivery)
	order := make([]string, 0)
	for _, item := range items {
		if _, exists := byUser[item.UserID]; !exists {
			order = append(order, item.UserID)
		}
		byUser[item.UserID] = append(byUser[item.UserID], item)
	}

	for userIndex, userID := range order {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		group := byUser[userID]
		batches := notificationDigestBatches(group)
		telegramID, err := strconv.ParseInt(userID, 10, 64)
		for batchIndex, batch := range batches {
			if err == nil {
				if waitErr := w.waitForTelegram(ctx, userID); waitErr != nil {
					return context.Cause(ctx)
				}
				activeItems := make([]domain.NotificationDelivery, 0, len(batch.Items))
				for _, item := range batch.Items {
					decision, checkErr := w.repository.DeliveryDecision(ctx, item.ID, item.ClaimToken)
					if checkErr != nil {
						return checkErr
					}
					switch decision {
					case repository.NotificationQueueGone:
						if discardErr := guard.discard(item.ID); discardErr != nil {
							return discardErr
						}
						continue
					case repository.NotificationQueueCancel:
						if cancelErr := guard.finish(item.ID, func(markCtx context.Context) error {
							return w.repository.MarkCancelled(markCtx, item.ID, item.ClaimToken)
						}); cancelErr != nil {
							return cancelErr
						}
						continue
					case repository.NotificationQueueDefer:
						if deferErr := guard.finish(item.ID, func(markCtx context.Context) error {
							return w.repository.MarkDeferred(markCtx, item.ID, item.ClaimToken)
						}); deferErr != nil {
							return deferErr
						}
						continue
					}
					activeItems = append(activeItems, item)
				}
				batch.Items = activeItems
				if len(activeItems) == 0 {
					continue
				}
				if leaseErr := context.Cause(ctx); leaseErr != nil {
					return leaseErr
				}
				_, err = w.bot.Send(&tele.User{ID: telegramID}, notificationDigestBatches(activeItems)[0].Text)
				w.limiter.Observe(err)
			}
			for _, item := range batch.Items {
				if markErr := guard.finish(item.ID, func(markCtx context.Context) error {
					if err == nil {
						return w.repository.MarkDelivered(markCtx, item.ID, item.ClaimToken)
					}
					return w.recordFailure(markCtx, item, err)
				}); markErr != nil {
					return markErr
				}
			}
			if err != nil {
				_, rateLimited := telegramlimit.FloodRetryAfter(err)
				for _, unsent := range batches[batchIndex+1:] {
					for _, item := range unsent.Items {
						if markErr := guard.finish(item.ID, func(markCtx context.Context) error {
							return w.recordFailure(markCtx, item, err)
						}); markErr != nil {
							return markErr
						}
					}
				}
				if rateLimited {
					for _, pendingUserID := range order[userIndex+1:] {
						for _, pending := range byUser[pendingUserID] {
							if markErr := guard.finish(pending.ID, func(markCtx context.Context) error {
								return w.recordFailure(markCtx, pending, err)
							}); markErr != nil {
								return markErr
							}
						}
					}
					return errTelegramRateLimited
				}
				break
			}
		}
	}
	return nil
}

func (w *NotificationWorker) deliverBotOutbox(ctx context.Context, item domain.BotOutboxDelivery, guard *notificationClaimGuard) error {
	if err := w.waitForTelegram(ctx, item.UserID); err != nil {
		return context.Cause(ctx)
	}
	decision, err := w.repository.BotOutboxDecision(ctx, item.ID, item.ClaimToken)
	if err != nil {
		return err
	}
	switch decision {
	case repository.NotificationQueueGone:
		return guard.discard(item.ID)
	case repository.NotificationQueueCancel:
		return guard.finish(item.ID, func(markCtx context.Context) error {
			return w.repository.MarkBotOutboxCancelled(markCtx, item.ID, item.ClaimToken)
		})
	case repository.NotificationQueueDefer:
		return guard.finish(item.ID, func(markCtx context.Context) error {
			return w.repository.MarkBotOutboxDeferred(markCtx, item.ID, item.ClaimToken)
		})
	}
	if err = context.Cause(ctx); err != nil {
		return err
	}

	if item.Kind == "lesson_reminder" {
		body, valid, refreshErr := w.refreshReminder(ctx, item, time.Now())
		if refreshErr != nil {
			return guard.finish(item.ID, func(markCtx context.Context) error { return w.recordBotOutboxFailure(markCtx, item, refreshErr) })
		}
		if !valid || item.ExpiresAt == nil || !time.Now().Before(*item.ExpiresAt) {
			return guard.finish(item.ID, func(markCtx context.Context) error {
				return w.repository.MarkBotOutboxCancelled(markCtx, item.ID, item.ClaimToken)
			})
		}
		item.Body = body
	}
	telegramID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err == nil {
		_, err = w.bot.Send(&tele.User{ID: telegramID}, item.Body)
		w.limiter.Observe(err)
	}
	rateLimited := false
	_, rateLimited = telegramlimit.FloodRetryAfter(err)
	if finishErr := guard.finish(item.ID, func(markCtx context.Context) error {
		if err == nil {
			return w.repository.MarkBotOutboxDelivered(markCtx, item.ID, item.ClaimToken)
		}
		return w.recordBotOutboxFailure(markCtx, item, err)
	}); finishErr != nil {
		return finishErr
	}
	if rateLimited {
		return err
	}
	return nil
}

func (w *NotificationWorker) recordBotOutboxFailure(
	ctx context.Context,
	item domain.BotOutboxDelivery,
	deliveryErr error,
) error {
	if retryAfter, limited := telegramlimit.FloodRetryAfter(deliveryErr); limited {
		markErr := w.repository.MarkBotOutboxRateLimited(
			ctx, item.ID, item.ClaimToken, retryAfter, deliveryErr,
		)
		if markErr != nil {
			slog.Error("notification worker: record bot outbox rate limit failed", "delivery_id", item.ID, "err", markErr)
			return markErr
		}
		slog.Warn("bot outbox delivery rate limited",
			"delivery_id", item.ID,
			"kind", item.Kind,
			"retry_after", retryAfter,
		)
		return nil
	}
	retryAfter, permanent := telegramRetryPolicy(deliveryErr, item.Attempts)
	var markErr error
	if permanent {
		markErr = w.repository.MarkBotOutboxPermanentFailure(ctx, item.ID, item.ClaimToken, deliveryErr)
	} else {
		markErr = w.repository.MarkBotOutboxFailed(ctx, item.ID, item.ClaimToken, item.Attempts, retryAfter, deliveryErr)
	}
	if markErr != nil {
		slog.Error("notification worker: record bot outbox failure failed", "delivery_id", item.ID, "err", markErr)
		return markErr
	}
	slog.Warn("bot outbox delivery failed",
		"delivery_id", item.ID,
		"kind", item.Kind,
		"attempt", item.Attempts,
		"permanent", permanent,
		"retry_after", retryAfter,
		"err", deliveryErr,
	)
	return nil
}

func (w *NotificationWorker) recordFailure(ctx context.Context, item domain.NotificationDelivery, deliveryErr error) error {
	if retryAfter, limited := telegramlimit.FloodRetryAfter(deliveryErr); limited {
		markErr := w.repository.MarkRateLimited(
			ctx, item.ID, item.ClaimToken, retryAfter, deliveryErr,
		)
		if markErr != nil {
			slog.Error("notification worker: record rate limit failed", "delivery_id", item.ID, "err", markErr)
			return markErr
		}
		slog.Warn("notification delivery rate limited",
			"delivery_id", item.ID,
			"retry_after", retryAfter,
		)
		return nil
	}
	retryAfter, permanent := telegramRetryPolicy(deliveryErr, item.Attempts)
	var markErr error
	if permanent {
		markErr = w.repository.MarkPermanentFailure(ctx, item.ID, item.ClaimToken, deliveryErr)
	} else {
		markErr = w.repository.MarkFailed(ctx, item.ID, item.ClaimToken, item.Attempts, retryAfter, deliveryErr)
	}
	if markErr != nil {
		slog.Error("notification worker: record failure failed", "delivery_id", item.ID, "err", markErr)
		return markErr
	}
	slog.Warn("notification delivery failed",
		"delivery_id", item.ID,
		"attempt", item.Attempts,
		"permanent", permanent,
		"retry_after", retryAfter,
		"err", deliveryErr,
	)
	return nil
}

type notificationMessage struct {
	Text  string
	Items []domain.NotificationDelivery
}

const notificationTelegramLimit = 4000

func notificationDigestBatches(items []domain.NotificationDelivery) []notificationMessage {
	if len(items) == 0 {
		return nil
	}
	const header = "🔔 Изменение расписания\n"
	batches := make([]notificationMessage, 0, 1)
	currentSections := make([]string, 0, len(items))
	currentItems := make([]domain.NotificationDelivery, 0, len(items))
	currentLength := len([]rune(header))

	flush := func() {
		if len(currentItems) == 0 {
			return
		}
		batches = append(batches, notificationMessage{
			Text:  header + strings.Join(currentSections, ""),
			Items: append([]domain.NotificationDelivery(nil), currentItems...),
		})
		currentSections = currentSections[:0]
		currentItems = currentItems[:0]
		currentLength = len([]rune(header))
	}

	for _, item := range items {
		summaryRunes := []rune(item.Summary)
		if len(summaryRunes) > 600 {
			summaryRunes = append(summaryRunes[:600], '…')
		}
		hint := "Откройте /settings и выберите эту группу, чтобы посмотреть актуальное расписание."
		if item.IsDefault {
			hint = "Откройте /week, чтобы посмотреть актуальное расписание."
		}
		section := fmt.Sprintf("\n%s · %s\n%s\n\n%s\n", item.UniversityName, item.GroupName, string(summaryRunes), hint)
		sectionLength := len([]rune(section))
		if len(currentItems) > 0 && currentLength+sectionLength > notificationTelegramLimit {
			flush()
		}
		currentSections = append(currentSections, section)
		currentItems = append(currentItems, item)
		currentLength += sectionLength
	}
	flush()
	return batches
}

func notificationText(item domain.NotificationDelivery) string {
	return notificationDigestBatches([]domain.NotificationDelivery{item})[0].Text
}

func notificationRetryDelay(attempt int) time.Duration {
	delay, _ := telegramRetryPolicy(nil, attempt)
	return delay
}

func telegramRetryPolicy(deliveryErr error, attempt int) (time.Duration, bool) {
	if retryAfter, limited := telegramlimit.FloodRetryAfter(deliveryErr); limited {
		return retryAfter, false
	}
	var apiErr *tele.Error
	if errors.As(deliveryErr, &apiErr) &&
		(apiErr.Code == 400 || apiErr.Code == 401 || apiErr.Code == 403 || apiErr.Code == 404) {
		return 0, true
	}
	switch attempt {
	case 1:
		return time.Minute, false
	case 2:
		return 5 * time.Minute, false
	case 3:
		return 30 * time.Minute, false
	default:
		return 2 * time.Hour, false
	}
}

func (w *NotificationWorker) waitForTelegram(ctx context.Context, recipient string) error {
	if w.backgroundLimiter != nil {
		if err := w.backgroundLimiter.WaitGlobal(ctx); err != nil {
			return err
		}
	}
	if w.waitSend != nil {
		return w.waitSend(ctx, recipient)
	}
	return w.limiter.Wait(ctx, recipient)
}

func (w *NotificationWorker) refreshReminder(ctx context.Context, item domain.BotOutboxDelivery, now time.Time) (string, bool, error) {
	if item.ExpiresAt == nil || !now.Before(*item.ExpiresAt) {
		return "", false, nil
	}
	var slot domain.ReminderContext
	if err := json.Unmarshal(item.ReminderContext, &slot); err != nil {
		return "", false, nil
	}
	if w.reminderSchedule == nil || w.reminderRecipient == nil {
		return "", false, fmt.Errorf("reminder delivery provider is unavailable")
	}
	recipient, err := w.reminderRecipient(ctx, item.UserID, item.GroupID)
	if err != nil || recipient == nil {
		return "", false, err
	}
	location, err := time.LoadLocation(recipient.Timezone)
	if err != nil {
		return "", false, err
	}
	date, err := time.ParseInLocation(time.DateOnly, slot.Date, location)
	if err != nil {
		return "", false, nil
	}
	startsAt, err := lessonStart(date, slot.TimeStart)
	if err != nil || !now.Before(startsAt) {
		return "", false, nil
	}
	if !startsAt.Equal(slot.StartsAt) || recipient.Subgroup != slot.Subgroup {
		return "", false, nil
	}
	if recipient.ReminderMinutes <= 0 || startsAt.Sub(now) > time.Duration(recipient.ReminderMinutes)*time.Minute {
		return "", false, nil
	}
	lessons, err := w.reminderSchedule.GetScheduleForGroup(ctx, item.GroupID, date)
	if err != nil {
		return "", false, err
	}
	current := reminderSlot{TimeStart: slot.TimeStart, TimeEnd: slot.TimeEnd}
	for _, lesson := range lessons {
		if lesson.TimeStart == slot.TimeStart && lesson.TimeEnd == slot.TimeEnd && (recipient.Subgroup == 0 || lesson.Subgroup == 0 || lesson.Subgroup == recipient.Subgroup) {
			current.Lessons = append(current.Lessons, lesson)
		}
	}
	if len(current.Lessons) == 0 {
		return "", false, nil
	}
	return reminderText(*recipient, date, current, startsAt.Sub(now)), true, nil
}
