package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	tele "gopkg.in/telebot.v3"
)

const notificationBatchSize = 25
const notificationRetention = 90 * 24 * time.Hour
const telegramGlobalInterval = 40 * time.Millisecond
const telegramRecipientInterval = 1100 * time.Millisecond

type notificationDeliveryRepository interface {
	ClaimPending(context.Context, int) ([]domain.NotificationDelivery, error)
	ClaimBotOutbox(context.Context, int) ([]domain.BotOutboxDelivery, error)
	RenewDeliveryClaims(context.Context, string, []string) error
	RenewBotOutboxClaims(context.Context, string, []string) error
	IsDeliveryActive(context.Context, string, string) (bool, error)
	IsBotOutboxActive(context.Context, string, string) (bool, error)
	MarkDelivered(context.Context, string, string) error
	MarkCancelled(context.Context, string, string) error
	MarkFailed(context.Context, string, string, int, time.Duration, error) error
	MarkPermanentFailure(context.Context, string, string, error) error
	MarkBotOutboxDelivered(context.Context, string, string) error
	MarkBotOutboxCancelled(context.Context, string, string) error
	MarkBotOutboxFailed(context.Context, string, string, int, time.Duration, error) error
	MarkBotOutboxPermanentFailure(context.Context, string, string, error) error
	PruneCompleted(context.Context, time.Duration) (int64, error)
}

type notificationSender interface {
	Send(tele.Recipient, interface{}, ...interface{}) (*tele.Message, error)
}

type NotificationWorker struct {
	repository         notificationDeliveryRepository
	bot                notificationSender
	interval           time.Duration
	claimRenewInterval time.Duration
	lastSend           time.Time
	recipients         map[string]time.Time
}

func NewNotificationWorker(
	repository *repository.NotificationRepository,
	bot *tele.Bot,
	interval time.Duration,
) *NotificationWorker {
	return &NotificationWorker{
		repository:         repository,
		bot:                bot,
		interval:           interval,
		claimRenewInterval: notificationClaimRenewInterval,
		recipients:         make(map[string]time.Time),
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
	items, err := w.repository.ClaimPending(ctx, notificationBatchSize)
	if err != nil {
		slog.Error("notification worker: claim failed", "err", err)
		return err
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
			return err
		}
	}

	outbox, err := w.repository.ClaimBotOutbox(ctx, notificationBatchSize)
	if err != nil {
		slog.Error("notification worker: claim bot outbox failed", "err", err)
		return err
	}
	if len(outbox) == 0 {
		return nil
	}
	ids := make([]string, len(outbox))
	for index, item := range outbox {
		ids[index] = item.ID
	}
	return w.withClaims(ctx, outbox[0].ClaimToken, ids, w.repository.RenewBotOutboxClaims,
		monitor, func(guard *notificationClaimGuard) error {
			for _, item := range outbox {
				if err := w.deliverBotOutbox(guard.ctx, item, guard); err != nil {
					return err
				}
			}
			return nil
		})
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

	for _, userID := range order {
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
					active, checkErr := w.repository.IsDeliveryActive(ctx, item.ID, item.ClaimToken)
					if checkErr != nil {
						return checkErr
					}
					if !active {
						if cancelErr := guard.finish(item.ID, func(markCtx context.Context) error {
							return w.repository.MarkCancelled(markCtx, item.ID, item.ClaimToken)
						}); cancelErr != nil {
							return cancelErr
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
				for _, unsent := range batches[batchIndex+1:] {
					for _, item := range unsent.Items {
						if markErr := guard.finish(item.ID, func(markCtx context.Context) error {
							return w.recordFailure(markCtx, item, err)
						}); markErr != nil {
							return markErr
						}
					}
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
	active, err := w.repository.IsBotOutboxActive(ctx, item.ID, item.ClaimToken)
	if err != nil {
		return err
	}
	if !active {
		return guard.finish(item.ID, func(markCtx context.Context) error {
			return w.repository.MarkBotOutboxCancelled(markCtx, item.ID, item.ClaimToken)
		})
	}
	if err = context.Cause(ctx); err != nil {
		return err
	}
	telegramID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err == nil {
		_, err = w.bot.Send(&tele.User{ID: telegramID}, item.Body)
	}
	return guard.finish(item.ID, func(markCtx context.Context) error {
		if err == nil {
			return w.repository.MarkBotOutboxDelivered(markCtx, item.ID, item.ClaimToken)
		}
		return w.recordBotOutboxFailure(markCtx, item, err)
	})
}

func (w *NotificationWorker) recordBotOutboxFailure(
	ctx context.Context,
	item domain.BotOutboxDelivery,
	deliveryErr error,
) error {
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
	const footer = "\nОткройте /week, чтобы посмотреть актуальное расписание."
	batches := make([]notificationMessage, 0, 1)
	currentSections := make([]string, 0, len(items))
	currentItems := make([]domain.NotificationDelivery, 0, len(items))
	currentLength := len([]rune(header)) + len([]rune(footer))

	flush := func() {
		if len(currentItems) == 0 {
			return
		}
		batches = append(batches, notificationMessage{
			Text:  header + strings.Join(currentSections, "") + footer,
			Items: append([]domain.NotificationDelivery(nil), currentItems...),
		})
		currentSections = currentSections[:0]
		currentItems = currentItems[:0]
		currentLength = len([]rune(header)) + len([]rune(footer))
	}

	for _, item := range items {
		summaryRunes := []rune(item.Summary)
		if len(summaryRunes) > 600 {
			summaryRunes = append(summaryRunes[:600], '…')
		}
		section := fmt.Sprintf("\n%s · %s\n%s\n", item.UniversityName, item.GroupName, string(summaryRunes))
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
	var flood tele.FloodError
	if errors.As(deliveryErr, &flood) {
		return time.Duration(flood.RetryAfter+1) * time.Second, false
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
	next := w.lastSend.Add(telegramGlobalInterval)
	if recipientNext := w.recipients[recipient].Add(telegramRecipientInterval); recipientNext.After(next) {
		next = recipientNext
	}
	if delay := time.Until(next); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	sentAt := time.Now()
	w.lastSend = sentAt
	w.recipients[recipient] = sentAt
	if len(w.recipients) > 10_000 {
		cutoff := sentAt.Add(-time.Hour)
		for id, at := range w.recipients {
			if at.Before(cutoff) {
				delete(w.recipients, id)
			}
		}
	}
	return nil
}
