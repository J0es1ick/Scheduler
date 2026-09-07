package handlers

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleMenu(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		slog.Error("finish dialog before main menu failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	return c.Send("Меню расписания:", keyboards.MainMenu())
}

func (h *Handler) HandleOpenMainMenu(c tele.Context) error {
	if h.privateTransientFlow(c) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if c.Callback() != nil {
		if err := h.leaveInlineForMenu(c); err != nil {
			return err
		}
	}
	return h.HandleMenu(c)
}

func (h *Handler) leaveInlineForMenu(c tele.Context) error {
	message := c.Message()
	isSchedule := h.hasTrackedScheduleMessages(c)
	if message != nil {
		isSchedule = isSchedule || message.Photo != nil
		if message.ReplyMarkup != nil {
			for _, row := range message.ReplyMarkup.InlineKeyboard {
				for _, button := range row {
					for _, action := range []string{"schedule_date", "schedule_week", "open_schedule_exports"} {
						if button.Unique == action || strings.HasPrefix(strings.TrimPrefix(button.Data, "\f"), action+"|") {
							isSchedule = true
						}
					}
				}
			}
		}
	}
	if !isSchedule {
		return h.retireCurrentInlineFlow(c, "Открыто главное меню.")
	}
	if err := c.Edit(&tele.ReplyMarkup{}); err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	h.scheduleMessagesMu.Lock()
	tracked := h.scheduleMessages[scheduleMessagesKey(c)]
	for _, id := range tracked.IDs {
		delete(h.scheduleMessages, strconv.FormatInt(c.Chat().ID, 10)+":"+strconv.Itoa(id))
	}
	h.scheduleMessagesMu.Unlock()
	return nil
}

func (h *Handler) HandleMore(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	return c.Send("Дополнительные разделы:", keyboards.MoreMenu())
}

func (h *Handler) HandleBackMore(c tele.Context) error {
	if h.privateTransientFlow(c) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if err := h.finishTransientFlow(c); err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	return editOrSend(c, "Дополнительные разделы:", keyboards.MoreMenu())
}

func (h *Handler) HandleCloseInline(c tele.Context) error {
	if h.privateTransientFlow(c) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	if err := h.retireCurrentInlineFlow(c, "Меню закрыто."); err != nil {
		return err
	}
	if isGroupChat(c) {
		return nil
	}
	return h.HandleMenu(c)
}

func (h *Handler) privateTransientFlow(c tele.Context) bool {
	if isGroupChat(c) || c.Sender() == nil || h.StateManager == nil {
		return false
	}
	current := h.StateManager.Get(c.Sender().ID)
	return current != nil && current.Step != "" && current.Step != "done"
}

func (h *Handler) HandleCancelUniversitySelection(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(current, "choosing_university", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте позже.")
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
		return editOrSend(c, "Настройка отменена. Для продолжения используйте /start.", nil)
	}
	if err = retireInlineMessage(c, "Смена вуза отменена."); err != nil {
		return err
	}
	return h.HandleMenu(c)
}

func (h *Handler) HandleCancelSearchType(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(current, "choosing_search_type", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте позже.")
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
		return editOrSend(c, "Поиск закрыт. Для продолжения используйте /start.", nil)
	}
	if err = retireInlineMessage(c, "Поиск закрыт."); err != nil {
		return err
	}
	return h.HandleMenu(c)
}

func (h *Handler) HandleCancelHotlineType(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(current, "choosing_hotline_type", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте позже.")
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
	}
	return editOrSend(c, "Дополнительные разделы:", keyboards.MoreMenu())
}

func (h *Handler) retireCurrentInlineFlow(c tele.Context, replacement string) error {
	if h.hasTrackedScheduleMessages(c) {
		return h.deleteTrackedScheduleMessages(c)
	}
	return retireInlineMessage(c, replacement)
}

func (h *Handler) HandleOpenWeekday(c tele.Context) error {
	from := time.Now()
	daysCount := 7
	args := callbackArguments(c)
	groupToken := ""
	if len(args) > 2 && args[2] != "" {
		ctx, cancel := reqCtx()
		defer cancel()
		target := h.scheduleCallbackTarget(ctx, c, args, 2)
		if target == nil {
			return nil
		}
		from = h.targetNow(ctx, target)
		groupToken = target.navigationReference()
	}
	if len(args) > 0 {
		parsed, err := parseScheduleDate(args[0], from.Location())
		if err == nil {
			from = parsed
		}
	}
	if len(args) > 1 {
		value, err := strconv.Atoi(args[1])
		if err == nil && value == 14 {
			daysCount = value
		}
	}
	_ = c.Respond()
	return editScheduleOverlay(c, "Выберите день недели:", keyboards.WeekDaySelector(from, daysCount, groupToken))
}

func (h *Handler) HandleOpenScheduleGroup(c tele.Context) error {
	_ = c.Respond()
	if h.hasTrackedScheduleMessages(c) {
		if err := h.deleteTrackedScheduleMessages(c); err != nil {
			return err
		}
		if isGroupChat(c) {
			return h.showChatSettings(c, false)
		}
		if err := h.finishTransientFlow(c); err != nil {
			return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
		}
		return h.showSubscriptionSettingsPage(c, false, 0)
	}
	if isGroupChat(c) {
		return h.HandleChatSettings(c)
	}
	if err := h.finishTransientFlow(c); err != nil {
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	return h.showSubscriptionSettingsPage(c, true, 0)
}

func (h *Handler) HandleAddSubscription(c tele.Context) error {
	return h.beginGroupChange(c, "subscriptions", callbackPage(c, 0))
}

func (h *Handler) HandleCancelGroupChange(c tele.Context) error {
	args := callbackArguments(c)
	destination := "main"
	nonceIndex := 1
	if len(args) > 0 && args[0] == "subscriptions" {
		destination = "subscriptions"
		nonceIndex = 2
	}
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) <= nonceIndex || !validFlow(current, "awaiting_query", args[nonceIndex]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile after cancelled group change failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось восстановить текущую группу. Используйте /start.")
	}
	if state == nil {
		h.StateManager.Delete(c.Sender().ID)
		return editOrSend(c, "Настройка группы отменена. Для продолжения используйте /start.", nil)
	}
	if destination == "subscriptions" {
		return h.showSubscriptionSettingsPage(c, true, callbackPage(c, 1))
	}
	if err = retireInlineMessage(c, "Смена группы отменена."); err != nil {
		return err
	}
	return h.HandleMenu(c)
}

func retireInlineMessage(c tele.Context, replacement string) error {
	if err := c.Delete(); err == nil {
		return nil
	}
	var err error
	emptyMarkup := &tele.ReplyMarkup{}
	if messageSupportsCaption(c.Message()) {
		err = c.EditCaption(replacement, emptyMarkup)
	} else {
		err = c.Edit(replacement, emptyMarkup)
	}
	if err == nil || strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (h *Handler) HandleBackUniversitySelection(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(current, "awaiting_query", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		slog.Error("restore profile before university selection failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось восстановить текущую группу. Попробуйте ещё раз позже.")
	}
	if state == nil {
		state = &dto.UserState{}
	}
	state.Step = "choosing_university"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		slog.Error("load universities for back navigation failed", "err", err)
		return c.Send("Не удалось загрузить список вузов. Попробуйте позже.")
	}
	return editOrSend(c, "Доступные вузы:", keyboards.UniversitySelector(universities, state.FlowNonce))
}

func (h *Handler) HandleShowSources(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		return err
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	text, err := h.sourcesInfoText(ctx)
	if err != nil {
		slog.Error("load public schedule sources failed", "err", err)
		return c.Send("Не удалось загрузить список источников. Попробуйте позже.")
	}
	return editOrSend(c, text, keyboards.BackToMoreMenu())
}

func (h *Handler) HandleOpenHotline(c tele.Context) error {
	_ = c.Respond()
	return h.HandleHotline(c)
}

func (h *Handler) HandleShowPrivacy(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		return err
	}
	_ = c.Respond()
	return editOrSend(c, privacyText(), keyboards.BackToMoreMenu())
}

func (h *Handler) HandleShowHelp(c tele.Context) error {
	_ = c.Respond()
	if err := h.finishTransientFlow(c); err != nil {
		return err
	}
	return editOrSend(c, compactHelpText, helpCategories(h.helpAdmin(c)))
}
