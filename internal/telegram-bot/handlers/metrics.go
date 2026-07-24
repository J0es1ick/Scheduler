package handlers

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

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

	sourceState := "все источники работают штатно"
	if metrics.SourcesStale+metrics.SourcesError+metrics.SourcesQuarantined > 0 {
		sourceState = "нужна проверка"
	}
	lastParse := "успешных запусков ещё не было"
	if metrics.LastSuccessfulParseAt != nil {
		lastParse = metrics.LastSuccessfulParseAt.In(time.Local).Format("02.01.2006 15:04")
	}

	lines := []string{
		"Метрики Scheduler",
		"",
		fmt.Sprintf("Пользователи: %d", metrics.Users),
		fmt.Sprintf("Подписки: %d", metrics.Subscriptions),
		fmt.Sprintf("Вузы: %d", metrics.Universities),
		fmt.Sprintf("Группы: %d", metrics.Groups),
		fmt.Sprintf("Занятия: %d", metrics.Lessons),
		"",
		fmt.Sprintf(
			"Источники: %d всего, %d в норме, %d обновляются, %d устарели, %d с ошибкой, %d в карантине — %s",
			metrics.SourcesTotal,
			metrics.SourcesHealthy,
			metrics.SourcesRunning,
			metrics.SourcesStale,
			metrics.SourcesError,
			metrics.SourcesQuarantined,
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
	}
	return c.Send(strings.Join(lines, "\n"))
}
