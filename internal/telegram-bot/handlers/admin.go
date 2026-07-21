package handlers

import (
	"fmt"
	"log/slog"

	"github.com/J0es1ick/Scheduler/internal/miniapp"
	telegram "gopkg.in/telebot.v3"
)

func (h *Handler) HandleAdmin(c telegram.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()

	userID := fmt.Sprint(c.Sender().ID)
	isAdmin, err := h.UserService.IsAdmin(ctx, userID)
	if err != nil {
		slog.Error("admin role check failed", "user_id", userID, "err", err)
		return c.Send("Не удалось проверить права доступа. Попробуйте позже.")
	}
	if !isAdmin {
		return c.Send("Эта команда доступна только администраторам.")
	}
	miniAppURL, err := miniapp.EditorURL(h.AdminPublicURL)
	if err != nil {
		slog.Warn("mini app URL is not configured", "err", err)
		return c.Send("Mini App пока не опубликован. Проверьте ADMIN_PUBLIC_URL.")
	}
	if err = miniapp.ConfigureMenu(c.Bot(), c.Sender(), h.AdminPublicURL, true); err != nil {
		slog.Warn("admin menu button configuration failed", "user_id", userID, "err", err)
	}

	menu := &telegram.ReplyMarkup{}
	button := menu.WebApp("Открыть редактор", &telegram.WebApp{URL: miniAppURL})
	menu.Inline(menu.Row(button))
	return c.Send("Админ-панель Scheduler", menu)
}
