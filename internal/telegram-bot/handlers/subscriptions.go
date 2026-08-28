package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleSettings(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
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
	if !item.IsActive {
		status += " · временно неактивна"
	}
	viewLabel := "Компактный текст"
	if item.ScheduleViewFormat == domain.ScheduleViewVisual {
		viewLabel = "Визуальная таблица"
	}
	subgroupLabel := "все"
	if item.Subgroup > 0 {
		subgroupLabel = strconv.Itoa(item.Subgroup)
	}
	return editOrSend(
		c,
		fmt.Sprintf("%s · %s\n\n%s\nФормат расписания: %s\nПодгруппа: %s", item.UniversityName, item.GroupName, status, viewLabel, subgroupLabel),
		keyboards.SubscriptionActions(item, page),
	)
}

func (h *Handler) HandleScheduleViewSettings(c tele.Context) error {
	groupID, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return h.settingsError(c, "load schedule view", err)
	}
	item, ok := findGroupSubscription(items, groupID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	_ = c.Respond()
	return editOrSend(
		c,
		fmt.Sprintf(
			"Формат расписания для %s · %s\n\nКомпактный — текстом в сообщении.\nТаблица — цветным изображением, похожим на расписание вуза.",
			item.UniversityName,
			item.GroupName,
		),
		keyboards.ScheduleViewSettings(item, callbackPage(c, 1)),
	)
}

func (h *Handler) HandleSetScheduleView(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) < 2 {
		return respondStaleCallback(c)
	}
	format := domain.ScheduleViewFormat(args[1])
	if format != domain.ScheduleViewCompact && format != domain.ScheduleViewVisual {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load schedule view", err)
	}
	item, ok := findGroupSubscription(items, args[0])
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	if err = h.SubscriptionService.SetGroupScheduleView(ctx, userID, item.GroupID, format); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
		}
		return h.settingsError(c, "set schedule view", err)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "Формат сохранён"})
	items, err = h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "reload schedule view", err)
	}
	item, ok = findGroupSubscription(items, args[0])
	if !ok {
		return respondStaleCallback(c)
	}
	return editOrSend(c, fmt.Sprintf(
		"Формат расписания для %s · %s сохранён.",
		item.UniversityName,
		item.GroupName,
	), keyboards.ScheduleViewSettings(item, callbackPage(c, 2)))
}

func (h *Handler) HandleSubgroupSettings(c tele.Context) error {
	groupReference, ok := callbackArgument(c)
	if !ok {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return h.settingsError(c, "load subgroup settings", err)
	}
	item, ok := findGroupSubscription(items, groupReference)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	_ = c.Respond()
	return editOrSend(
		c,
		fmt.Sprintf("Подгруппа для %s · %s\n\nОбщие занятия будут показаны при любом выборе.", item.UniversityName, item.GroupName),
		keyboards.SubgroupSettings(item, callbackPage(c, 1)),
	)
}

func (h *Handler) HandleSetSubscriptionSubgroup(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) < 2 {
		return respondStaleCallback(c)
	}
	subgroup, err := strconv.Atoi(args[1])
	if err != nil || subgroup < 0 || subgroup > 100 {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load subgroup settings", err)
	}
	item, ok := findGroupSubscription(items, args[0])
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	if err = h.SubscriptionService.SetGroupSubgroup(ctx, userID, item.GroupID, subgroup); err != nil {
		return h.settingsError(c, "set subgroup", err)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "Подгруппа сохранена"})
	items, err = h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "reload subgroup settings", err)
	}
	item, ok = findGroupSubscription(items, args[0])
	if !ok {
		return respondStaleCallback(c)
	}
	return editOrSend(c, "Настройка подгруппы сохранена.", keyboards.SubgroupSettings(item, callbackPage(c, 2)))
}

func (h *Handler) HandleSubscriptionSchedule(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) < 2 {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		return h.settingsError(c, "load subscription schedule", err)
	}
	item, ok := findGroupSubscription(items, args[0])
	if !ok || !item.IsActive {
		return c.Respond(&tele.CallbackResponse{Text: "Группа сейчас недоступна", ShowAlert: true})
	}
	_ = c.Respond()
	target := &scheduleTarget{
		GroupID: item.GroupID, GroupName: item.GroupName,
		UniversityID: item.UniversityID, University: item.UniversityName,
		ViewFormat: item.ScheduleViewFormat, Subgroup: item.Subgroup,
	}
	from := h.targetNow(ctx, target)
	daysCount := 1
	switch args[1] {
	case "tomorrow":
		from = from.AddDate(0, 0, 1)
	case "week":
		from = scheduleWeekStart(from)
		daysCount = 7
	case "today":
	default:
		return respondStaleCallback(c)
	}
	days, err := h.getScheduleForTarget(ctx, target, from, from.AddDate(0, 0, daysCount-1))
	if err != nil {
		return sendScheduleLoadError(c, err)
	}
	var markup *tele.ReplyMarkup
	if daysCount == 1 {
		markup = keyboards.ScheduleDayNavigation(from, target.GroupName, false, target.GroupID)
	} else {
		markup = keyboards.ScheduleWeekNavigation(from, target.GroupName, false, target.GroupID, daysCount)
	}
	return h.sendScheduleView(
		ctx, c, days, target, from, daysCount,
		markup,
		formatSchedulePeriodHTML(from, daysCount),
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
	state := h.StateManager.Get(c.Sender().ID)
	if state == nil {
		state = &dto.UserState{Step: "done"}
	}
	state.PendingSubscriptionDeleteToken = newFlowNonce()
	state.PendingSubscriptionDeleteGroupID = item.GroupID
	state.PendingSubscriptionDeleteExpiresAt = time.Now().Add(10 * time.Minute)
	h.StateManager.Set(c.Sender().ID, state)
	return editOrSend(
		c,
		fmt.Sprintf("Удалить подписку на %s · %s?%s", item.UniversityName, item.GroupName, warning),
		keyboards.DeleteSubscriptionConfirmation(
			keyboards.GroupToken(item.GroupID),
			page,
			state.PendingSubscriptionDeleteToken,
		),
	)
}

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
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load subscription", err)
	}
	item, ok := findGroupSubscription(items, groupID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	if !item.IsActive {
		return c.Respond(&tele.CallbackResponse{Text: "Неактивную группу нельзя назначить основной", ShowAlert: true})
	}
	_ = c.Respond()
	if err = h.SubscriptionService.SetDefaultGroup(ctx, userID, item.GroupID); err != nil {
		return h.settingsError(c, "set default group", err)
	}
	if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return h.settingsError(c, "restore profile", err)
	}
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
}

func (h *Handler) HandleConfirmDeleteSubscription(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) < 3 {
		return respondStaleCallback(c)
	}
	groupID := args[0]
	state := h.StateManager.Get(c.Sender().ID)
	if !consumeSubscriptionDeleteIntent(state, args[2], groupID, time.Now()) {
		return c.Respond(&tele.CallbackResponse{Text: "Подтверждение устарело", ShowAlert: true})
	}
	h.StateManager.Set(c.Sender().ID, state)
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	items, err := h.SubscriptionService.GetGroupSubscriptions(ctx, userID)
	if err != nil {
		return h.settingsError(c, "load subscriptions", err)
	}
	item, ok := findGroupSubscription(items, groupID)
	if !ok {
		return c.Respond(&tele.CallbackResponse{Text: "Подписка уже удалена"})
	}
	groupID = item.GroupID

	newDefault, err := h.SubscriptionService.UnsubscribeAndSelectDefault(ctx, userID, groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return h.settingsError(c, "delete subscription", err)
	}
	if newDefault == "" {
		h.StateManager.Delete(c.Sender().ID)
	} else if _, _, err = h.restoreProfile(ctx, c.Sender().ID); err != nil {
		return h.settingsError(c, "restore profile", err)
	}
	return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
}

func (h *Handler) HandleCancelDeleteSubscription(c tele.Context) error {
	args := callbackArguments(c)
	state := h.StateManager.Get(c.Sender().ID)
	if len(args) < 3 || !consumeSubscriptionDeleteIntent(state, args[2], args[0], time.Now()) {
		return c.Respond(&tele.CallbackResponse{Text: "Подтверждение уже недействительно"})
	}
	h.StateManager.Set(c.Sender().ID, state)
	return h.HandleOpenSubscription(c)
}

func consumeSubscriptionDeleteIntent(state *dto.UserState, token, groupReference string, now time.Time) bool {
	if state == nil || token == "" || state.PendingSubscriptionDeleteToken != token ||
		!state.PendingSubscriptionDeleteExpiresAt.After(now) ||
		(keyboards.GroupToken(state.PendingSubscriptionDeleteGroupID) != groupReference &&
			state.PendingSubscriptionDeleteGroupID != groupReference) {
		return false
	}
	state.PendingSubscriptionDeleteToken = ""
	state.PendingSubscriptionDeleteGroupID = ""
	state.PendingSubscriptionDeleteExpiresAt = time.Time{}
	return true
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
		return editOrSend(c, text, markup)
	}
	return c.Send(text, markup)
}

func findGroupSubscription(
	items []domain.GroupSubscription,
	groupReference string,
) (domain.GroupSubscription, bool) {
	for _, item := range items {
		if item.GroupID == groupReference || keyboards.GroupToken(item.GroupID) == groupReference {
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
