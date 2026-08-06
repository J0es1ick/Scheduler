package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSettings(c tele.Context) error {
	return h.showSubscriptionSettingsPage(c, false, 0)
}

func (h *Handler) HandleSubscriptionPage(c tele.Context) error {
	defer c.Respond()
	page := 0
	if value, ok := callbackArgument(c); ok {
		page, _ = strconv.Atoi(value)
	}
	return h.showSubscriptionSettingsPage(c, true, page)
}

func (h *Handler) HandleOpenSubscription(c tele.Context) error {
	groupID, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return h.settingsError(c, "load subscriptions", err)
	}
	item, ok := findGroupSubscription(items, groupID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	_ = c.Respond()
	page := callbackPage(c, 1)
	status := "Дополнительная группа"
	if item.IsDefault {
		status = "Основная группа"
	}
	return editOrSend(
		c,
		fmt.Sprintf("%s · %s\n\n%s", item.UniversityName, item.GroupName, status),
		keyboards.SubscriptionActions(item, page),
	)
}

func (h *Handler) HandleRequestDeleteSubscription(c tele.Context) error {
	groupID, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return h.settingsError(c, "load subscriptions", err)
	}
	item, ok := findGroupSubscription(items, groupID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	_ = c.Respond()
	page := callbackPage(c, 1)
	warning := ""
	if item.IsDefault {
		warning = "\n\nПосле удаления основной станет следующая группа из списка."
	}
	return editOrSend(
		c,
		fmt.Sprintf("Удалить подписку на %s · %s?%s", item.UniversityName, item.GroupName, warning),
		keyboards.DeleteSubscriptionConfirmation(groupID, page),
	)
}

// HandleDeleteSubscription keeps old callback messages safe by showing the
// confirmation instead of performing an immediate deletion.
func (h *Handler) HandleDeleteSubscription(c tele.Context) error {
	return h.HandleRequestDeleteSubscription(c)
}

func (h *Handler) HandleSetDefaultSubscription(c tele.Context) error {
	groupID, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()

	userID := fmt.Sprint(c.Sender().ID)
	subscribed, err := h.SubscriptionService.HasGroupSubscription(ctx, userID, groupID)
	if err != nil {
		return h.settingsError(c, "check subscription", err)
	}
	if !subscribed {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	_ = c.Respond()
	if err = h.UserService.SetDefaultGroup(ctx, userID, groupID); err != nil {
		return h.settingsError(c, "set default group", err)
	}
	if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return h.settingsError(c, "restore profile", err)
	}
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
}

func (h *Handler) HandleConfirmDeleteSubscription(c tele.Context) error {
	groupID, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
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
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
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
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 0))
}

func (h *Handler) showSubscriptionSettingsPage(c tele.Context, edit bool, page int) error {
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

	text := subscriptionSettingsText(
		items,
		user.NotificationsEnabled,
		user.ReminderEnabled,
		user.ReminderMinutes,
	)
	markup := keyboards.SubscriptionSettings(
		items,
		user.NotificationsEnabled,
		user.ReminderEnabled,
		user.ReminderMinutes,
		page,
	)
	if edit {
		if err = c.Edit(text, markup); err == nil || strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		slog.Debug("edit subscription settings failed; sending new message", "user_id", userID, "err", err)
	}
	return c.Send(text, markup)
}

func findGroupSubscription(
	items []domain.GroupSubscription,
	groupID string,
) (domain.GroupSubscription, bool) {
	for _, item := range items {
		if item.GroupID == groupID {
			return item, true
		}
	}
	return domain.GroupSubscription{}, false
}

func subscriptionSettingsText(
	items []domain.GroupSubscription,
	notificationsEnabled bool,
	reminderEnabled bool,
	reminderMinutes int,
) string {
	status := "включены"
	if !notificationsEnabled {
		status = "выключены"
	}
	var builder strings.Builder
	reminderStatus := "выключены"
	if reminderEnabled {
		reminderStatus = fmt.Sprintf(
			"за %d мин. до пары основной группы",
			reminderMinutes,
		)
	}
	fmt.Fprintf(
		&builder,
		"Мои группы\n\nУведомления: %s\nНапоминания: %s\nПодписок: %d\n",
		status,
		reminderStatus,
		len(items),
	)
	if len(items) == 0 {
		builder.WriteString("\nНет выбранных групп. Нажмите «Добавить группу» ниже.")
		return builder.String()
	}
	builder.WriteString("\n● — основная группа для команд расписания. Нажмите на группу, чтобы открыть её настройки.")
	return builder.String()
}

func callbackArgument(c tele.Context) (string, bool) {
	args := callbackArguments(c)
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", false
	}
	return args[0], true
}

func callbackPage(c tele.Context, position int) int {
	args := callbackArguments(c)
	if position < 0 || position >= len(args) {
		return 0
	}
	page, err := strconv.Atoi(args[position])
	if err != nil || page < 0 {
		return 0
	}
	return page
}

func callbackArguments(c tele.Context) []string {
	args := c.Args()
	if c.Callback() == nil {
		return args
	}
	return normalizeCallbackArguments(args)
}

func normalizeCallbackArguments(args []string) []string {
	// Compatibility with schedule buttons sent by the short-lived version
	// that retried an already processed Telebot markup. After Telebot removes
	// the outer endpoint, such callbacks start with another form-feed endpoint
	// followed by the real arguments.
	if len(args) > 0 && strings.HasPrefix(args[0], "\f") {
		return args[1:]
	}
	return args
}

func respondStaleCallback(c tele.Context) error {
	return c.Respond(&tele.CallbackResponse{Text: "Меню устарело, откройте его снова"})
}

func (h *Handler) settingsError(c tele.Context, operation string, err error) error {
	if err == nil {
		err = errors.New("user not found")
	}
	slog.Error("subscription settings failed", "operation", operation, "user_id", c.Sender().ID, "err", err)
	return c.Send("Не удалось обновить настройки. Попробуйте ещё раз позже.")
}
