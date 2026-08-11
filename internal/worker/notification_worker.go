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

type NotificationWorker struct {
	repository *repository.NotificationRepository
	bot        *tele.Bot
	interval   time.Duration
	lastSend   time.Time
	recipients map[string]time.Time
}

func NewNotificationWorker(
	repository *repository.NotificationRepository,
	bot *tele.Bot,
	interval time.Duration,
) *NotificationWorker {
	return &NotificationWorker{
		repository: repository,
		bot:        bot,
		interval:   interval,
		recipients: make(map[string]time.Time),
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
	w.tick(ctx)
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
			w.tick(ctx)
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

func (w *NotificationWorker) tick(ctx context.Context) {
	items, err := w.repository.ClaimPending(ctx, notificationBatchSize)
	if err != nil {
		slog.Error("notification worker: claim failed", "err", err)
		return
	}
	w.deliverScheduleBatch(ctx, items)

	outbox, err := w.repository.ClaimBotOutbox(ctx, notificationBatchSize)
	if err != nil {
		slog.Error("notification worker: claim bot outbox failed", "err", err)
		return
	}
	for _, item := range outbox {
		if ctx.Err() != nil {
			return
		}
		w.deliverBotOutbox(ctx, item)
	}
}

func (w *NotificationWorker) deliverScheduleBatch(ctx context.Context, items []domain.NotificationDelivery) {
	byUser := make(map[string][]domain.NotificationDelivery)
	order := make([]string, 0)
	for _, item := range items {
		active, err := w.repository.IsDeliveryActive(ctx, item.ID)
		if err != nil {
			w.recordFailure(ctx, item, err)
			continue
		}
		if !active {
			if err = w.repository.MarkCancelled(ctx, item.ID); err != nil {
				slog.Error("notification worker: cancel ineligible delivery failed", "delivery_id", item.ID, "err", err)
			}
			continue
		}
		if _, exists := byUser[item.UserID]; !exists {
			order = append(order, item.UserID)
		}
		byUser[item.UserID] = append(byUser[item.UserID], item)
	}

	for _, userID := range order {
		if ctx.Err() != nil {
			return
		}
		group := byUser[userID]
		batches := notificationDigestBatches(group)
		telegramID, err := strconv.ParseInt(userID, 10, 64)
		for batchIndex, batch := range batches {
			if err == nil {
				if err = w.waitForTelegram(ctx, userID); err == nil {
					_, err = w.bot.Send(&tele.User{ID: telegramID}, batch.Text)
				}
			}
			for _, item := range batch.Items {
				if err == nil {
					if markErr := w.repository.MarkDelivered(ctx, item.ID); markErr != nil {
						slog.Error("notification worker: mark delivered failed", "delivery_id", item.ID, "err", markErr)
					}
				} else {
					w.recordFailure(ctx, item, err)
				}
			}
			if err != nil {
				for _, unsent := range batches[batchIndex+1:] {
					for _, item := range unsent.Items {
						w.recordFailure(ctx, item, err)
					}
				}
				break
			}
		}
	}
}

func (w *NotificationWorker) deliverBotOutbox(ctx context.Context, item domain.BotOutboxDelivery) {
	active, err := w.repository.IsBotOutboxActive(ctx, item.ID)
	if err != nil {
		w.recordBotOutboxFailure(ctx, item, err)
		return
	}
	if !active {
		if err = w.repository.MarkBotOutboxCancelled(ctx, item.ID); err != nil {
			slog.Error("notification worker: cancel ineligible bot outbox failed", "delivery_id", item.ID, "err", err)
		}
		return
	}
	telegramID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err == nil {
		if err = w.waitForTelegram(ctx, item.UserID); err == nil {
			_, err = w.bot.Send(&tele.User{ID: telegramID}, item.Body)
		}
	}
	if err == nil {
		if markErr := w.repository.MarkBotOutboxDelivered(ctx, item.ID); markErr != nil {
			slog.Error("notification worker: mark bot outbox delivered failed", "delivery_id", item.ID, "err", markErr)
		}
		return
	}
	w.recordBotOutboxFailure(ctx, item, err)
}

func (w *NotificationWorker) recordBotOutboxFailure(
	ctx context.Context,
	item domain.BotOutboxDelivery,
	deliveryErr error,
) {
	retryAfter, permanent := telegramRetryPolicy(deliveryErr, item.Attempts)
	var markErr error
	if permanent {
		markErr = w.repository.MarkBotOutboxPermanentFailure(ctx, item.ID, deliveryErr)
	} else {
		markErr = w.repository.MarkBotOutboxFailed(ctx, item.ID, item.Attempts, retryAfter, deliveryErr)
	}
	if markErr != nil {
		slog.Error("notification worker: record bot outbox failure failed", "delivery_id", item.ID, "err", markErr)
		return
	}
	slog.Warn("bot outbox delivery failed",
		"delivery_id", item.ID,
		"kind", item.Kind,
		"attempt", item.Attempts,
		"permanent", permanent,
		"retry_after", retryAfter,
		"err", deliveryErr,
	)
}

func (w *NotificationWorker) recordFailure(ctx context.Context, item domain.NotificationDelivery, deliveryErr error) {
	retryAfter, permanent := telegramRetryPolicy(deliveryErr, item.Attempts)
	var markErr error
	if permanent {
		markErr = w.repository.MarkPermanentFailure(ctx, item.ID, deliveryErr)
	} else {
		markErr = w.repository.MarkFailed(ctx, item.ID, item.Attempts, retryAfter, deliveryErr)
	}
	if markErr != nil {
		slog.Error("notification worker: record failure failed", "delivery_id", item.ID, "err", markErr)
		return
	}
	slog.Warn("notification delivery failed",
		"delivery_id", item.ID,
		"attempt", item.Attempts,
		"permanent", permanent,
		"retry_after", retryAfter,
		"err", deliveryErr,
	)
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
