package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSettings(c tele.Context) error {
	return h.showSubscriptionSettings(c, false)
}

func (h *Handler) HandleSetDefaultSubscription(c tele.Context) error {
	defer c.Respond()
	groupID, ok := callbackArgument(c)
	if !ok {
		return c.Send("Не удалось определить группу.")
	}
	ctx, cancel := reqCtx()
	defer cancel()

	userID := fmt.Sprint(c.Sender().ID)
	subscribed, err := h.SubscriptionService.HasGroupSubscription(ctx, userID, groupID)
	if err != nil {
		return h.settingsError(c, "check subscription", err)
	}
	if !subscribed {
		return c.Send("Этой подписки уже нет. Обновите настройки.")
	}
	if err = h.UserService.SetDefaultGroup(ctx, userID, groupID); err != nil {
		return h.settingsError(c, "set default group", err)
	}
	if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return h.settingsError(c, "restore profile", err)
	}
	return h.showSubscriptionSettings(c, true)
}

func (h *Handler) HandleDeleteSubscription(c tele.Context) error {
	defer c.Respond()
	groupID, ok := callbackArgument(c)
	if !ok {
		return c.Send("Не удалось определить группу.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)

	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil || user == nil {
		return h.settingsError(c, "load user", err)
	}
	if err = h.SubscriptionService.Unsubscribe(ctx, userID, groupID, "group"); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return h.settingsError(c, "delete subscription", err)
		}
	}

	if user.DefaultGroupID == groupID {
		remaining, loadErr := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
		if loadErr != nil {
			return h.settingsError(c, "load remaining subscriptions", loadErr)
		}
		newDefault := ""
		if len(remaining) > 0 {
			newDefault = remaining[0].GroupID
		}
		if err = h.UserService.SetDefaultGroup(ctx, userID, newDefault); err != nil {
			return h.settingsError(c, "replace default group", err)
		}
		if newDefault == "" {
			h.StateManager.Delete(c.Sender().ID)
		} else if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
			return h.settingsError(c, "restore profile", err)
		}
	}
	return h.showSubscriptionSettings(c, true)
}

func (h *Handler) HandleToggleNotifications(c tele.Context) error {
	defer c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)

	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil || user == nil {
		return h.settingsError(c, "load user", err)
	}
	if err = h.UserService.SetNotificationsEnabled(ctx, userID, !user.NotificationsEnabled); err != nil {
		return h.settingsError(c, "toggle notifications", err)
	}
	return h.showSubscriptionSettings(c, true)
}

func (h *Handler) showSubscriptionSettings(c tele.Context, edit bool) error {
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	user, err := h.UserService.GetUser(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load user", err)
	}
	if user == nil {
		return c.Send("Сначала запустите бота: /start")
	}
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load subscriptions", err)
	}

	text := subscriptionSettingsText(items, user.NotificationsEnabled)
	markup := keyboards.SubscriptionSettings(items, user.NotificationsEnabled)
	if edit {
		if err = c.Edit(text, markup); err == nil || strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		slog.Debug("edit subscription settings failed; sending new message", "user_id", userID, "err", err)
	}
	return c.Send(text, markup)
}

func subscriptionSettingsText(items []domain.GroupSubscription, notificationsEnabled bool) string {
	status := "включены"
	if !notificationsEnabled {
		status = "выключены"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "Настройки\n\nУведомления: %s\nПодписок: %d\n", status, len(items))
	if len(items) == 0 {
		builder.WriteString("\nНет выбранных групп. Используйте /change_group, чтобы добавить основную группу.")
		return builder.String()
	}
	builder.WriteString("\n● — основная группа для команд расписания. Нажмите на другую группу, чтобы сделать её основной.")
	return builder.String()
}

func callbackArgument(c tele.Context) (string, bool) {
	args := c.Args()
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", false
	}
	return args[0], true
}

func (h *Handler) settingsError(c tele.Context, operation string, err error) error {
	if err == nil {
		err = errors.New("user not found")
	}
	slog.Error("subscription settings failed", "operation", operation, "user_id", c.Sender().ID, "err", err)
	return c.Send("Не удалось обновить настройки. Попробуйте ещё раз позже.")
}
