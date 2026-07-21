package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	tele "gopkg.in/telebot.v3"
)

const notificationBatchSize = 25
const notificationRetention = 90 * 24 * time.Hour

type NotificationWorker struct {
	repository *repository.NotificationRepository
	bot        *tele.Bot
	interval   time.Duration
}

func NewNotificationWorker(
	repository *repository.NotificationRepository,
	bot *tele.Bot,
	interval time.Duration,
) *NotificationWorker {
	return &NotificationWorker{repository: repository, bot: bot, interval: interval}
}

func (w *NotificationWorker) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *NotificationWorker) run(ctx context.Context) {
	slog.Info("notification worker started", "interval", w.interval)
	w.prune(ctx)
	w.tick(ctx)

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
		case <-pruneTicker.C:
			w.prune(ctx)
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
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		w.deliver(ctx, item)
	}
}

func (w *NotificationWorker) deliver(ctx context.Context, item domain.NotificationDelivery) {
	active, err := w.repository.IsDeliveryActive(ctx, item.ID)
	if err != nil {
		w.recordFailure(ctx, item, err)
		return
	}
	if !active {
		if err = w.repository.MarkCancelled(ctx, item.ID); err != nil {
			slog.Error("notification worker: cancel ineligible delivery failed", "delivery_id", item.ID, "err", err)
		}
		return
	}
	telegramID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err == nil {
		_, err = w.bot.Send(&tele.User{ID: telegramID}, notificationText(item))
	}
	if err == nil {
		if markErr := w.repository.MarkDelivered(ctx, item.ID); markErr != nil {
			slog.Error("notification worker: mark delivered failed", "delivery_id", item.ID, "err", markErr)
		}
		return
	}

	w.recordFailure(ctx, item, err)
}

func (w *NotificationWorker) recordFailure(ctx context.Context, item domain.NotificationDelivery, deliveryErr error) {
	retryAfter := notificationRetryDelay(item.Attempts)
	if markErr := w.repository.MarkFailed(ctx, item.ID, item.Attempts, retryAfter, deliveryErr); markErr != nil {
		slog.Error("notification worker: record failure failed", "delivery_id", item.ID, "err", markErr)
		return
	}
	slog.Warn("notification delivery failed",
		"delivery_id", item.ID,
		"attempt", item.Attempts,
		"retry_after", retryAfter,
		"err", deliveryErr,
	)
}

func notificationText(item domain.NotificationDelivery) string {
	return fmt.Sprintf(
		"🔔 Изменение расписания\n\n%s · %s\n%s\n\nОткройте /week, чтобы посмотреть актуальное расписание.",
		item.UniversityName,
		item.GroupName,
		item.Summary,
	)
}

func notificationRetryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 30 * time.Minute
	default:
		return 2 * time.Hour
	}
}
