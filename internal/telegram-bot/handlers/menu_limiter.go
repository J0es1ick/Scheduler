package handlers

import (
	"context"
	"strconv"

	"github.com/J0es1ick/Scheduler/internal/miniapp"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) configureMiniAppMenu(
	ctx context.Context,
	bot *tele.Bot,
	user *tele.User,
	isAdmin bool,
) error {
	if h.TelegramLimiter == nil {
		return miniapp.ConfigureMenu(bot, user, h.AdminPublicURL, isAdmin)
	}
	recipient := strconv.FormatInt(user.ID, 10)
	if err := h.TelegramLimiter.Wait(ctx, recipient); err != nil {
		return err
	}
	err := miniapp.ConfigureMenu(bot, user, h.AdminPublicURL, isAdmin)
	if _, limited := h.TelegramLimiter.Observe(err); !limited {
		return err
	}
	if err = h.TelegramLimiter.Wait(ctx, recipient); err != nil {
		return err
	}
	err = miniapp.ConfigureMenu(bot, user, h.AdminPublicURL, isAdmin)
	h.TelegramLimiter.Observe(err)
	return err
}
