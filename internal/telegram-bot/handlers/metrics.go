package handlers

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleMetrics(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()

	userID := fmt.Sprint(c.Sender().ID)
	isAdmin, err := h.UserService.IsAdmin(ctx, userID)
	if err != nil {
		slog.Error("metrics role check failed", "user_id", userID, "err", err)
		return c.Send("Не удалось проверить права доступа. Попробуйте позже.")
	}
	if !isAdmin {
		return c.Send("Эта команда доступна только администраторам.")
	}

	metrics, err := h.MetricsService.Get(ctx)
	if err != nil {
		slog.Error("load bot metrics failed", "user_id", userID, "err", err)
		return c.Send("Не удалось загрузить метрики сервиса. Попробуйте позже.")
	}

	return c.Send(formatServiceMetrics(metrics))
}

func formatServiceMetrics(metrics *domain.ServiceMetrics) string {
	sourceState := "все источники работают штатно"
	if metrics.SourcesStale+metrics.SourcesError+metrics.SourcesQuarantined > 0 {
		sourceState = "нужна проверка"
	}
	lastParse := "успешных запусков ещё не было"
	if metrics.LastSuccessfulParseAt != nil {
		lastParse = metrics.LastSuccessfulParseAt.In(time.Local).Format("02.01.2006 15:04")
	}
	reminderState := "ещё не запускался"
	if metrics.ReminderWorker.LastFinishedAt != nil {
		reminderState = fmt.Sprintf(
			"%s · получателей: %d · ошибок: %d · %.1f сек.",
			metrics.ReminderWorker.LastFinishedAt.In(time.Local).Format("02.01.2006 15:04:05"),
			metrics.ReminderWorker.LastProcessed,
			metrics.ReminderWorker.LastFailures,
			float64(metrics.ReminderWorker.LastDurationMS)/1000,
		)
		if metrics.ReminderWorker.Cursor != "" {
			reminderState += " · обход будет продолжен"
		}
	}
	lastReminderCycle := "полный обход ещё не завершался"
	if metrics.ReminderWorker.LastFullCycleAt != nil {
		lastReminderCycle = metrics.ReminderWorker.LastFullCycleAt.
			In(time.Local).
			Format("02.01.2006 15:04:05")
	}

	lines := []string{
		"Метрики Scheduler",
		"",
		fmt.Sprintf("Пользователи: %d", metrics.Users),
		fmt.Sprintf("Подписки: %d", metrics.Subscriptions),
		fmt.Sprintf("Вузы: %d", metrics.Universities),
		fmt.Sprintf("Группы: %d", metrics.Groups),
		fmt.Sprintf("Занятия: %d", metrics.Lessons),
		fmt.Sprintf(
			"Хранилище: БД %s, таблица снимков %s, таблица загрузок коннекторов %s",
			formatMetricBytes(metrics.DatabaseBytes),
			formatMetricBytes(metrics.SnapshotPayloadBytes),
			formatMetricBytes(metrics.ConnectorPayloadBytes),
		),
		"",
		fmt.Sprintf(
			"Источники: %d всего, %d в норме, %d обновляются, %d устарели, %d с ошибкой, %d в карантине, %d отключено — %s",
			metrics.SourcesTotal,
			metrics.SourcesHealthy,
			metrics.SourcesRunning,
			metrics.SourcesStale,
			metrics.SourcesError,
			metrics.SourcesQuarantined,
			metrics.SourcesDisabled,
			sourceState,
		),
		fmt.Sprintf(
			"Очереди: %d уведомлений и %d служебных сообщений ожидают отправки",
			metrics.PendingNotifications,
			metrics.PendingOutbox,
		),
		fmt.Sprintf(
			"Ошибки доставки: %d уведомлений, %d служебных сообщений",
			metrics.FailedNotifications,
			metrics.FailedOutbox,
		),
		"Последний успешный парсинг: " + lastParse,
		"Worker напоминаний: " + reminderState,
		"Последний полный обход напоминаний: " + lastReminderCycle,
	}
	if metrics.ReminderWorker.LastError != "" {
		lines = append(lines, "Последняя ошибка напоминаний: "+metrics.ReminderWorker.LastError)
	}
	return strings.Join(lines, "\n")
}

func formatMetricBytes(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d Б", value)
	}
	if value < unit*unit {
		return fmt.Sprintf("%.1f КиБ", float64(value)/float64(unit))
	}
	if value < unit*unit*unit {
		return fmt.Sprintf("%.1f МиБ", float64(value)/float64(unit*unit))
	}
	return fmt.Sprintf("%.1f ГиБ", float64(value)/float64(unit*unit*unit))
}
