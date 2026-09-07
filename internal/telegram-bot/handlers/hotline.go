package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleHotline(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	if _, err := h.UserService.RegisterOrGetUser(ctx, fmt.Sprint(c.Sender().ID), c.Sender().Username); err != nil {
		slog.Error("hotline register user failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось открыть форму обращения. Попробуйте позже.")
	}
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось открыть форму обращения. Попробуйте позже.")
	}
	if state == nil {
		state = &dto.UserState{}
	}
	state.Step = "choosing_hotline_type"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	text := "Горячая линия\n\nСообщите об ошибке расписания, предложите новое учебное заведение или отправьте пожелание о работе бота. Выберите тип обращения:"
	if c.Callback() != nil {
		return editOrSend(c, text, keyboards.HotlineTypeSelector(state.FlowNonce))
	}
	return c.Send(text, keyboards.HotlineTypeSelector(state.FlowNonce))
}

func (h *Handler) HandleHotlineType(c tele.Context) error {
	args := callbackArguments(c)
	if len(args) < 2 || (args[0] != domain.SupportRequestUpdateExisting && args[0] != domain.SupportRequestNewInstitution && args[0] != domain.SupportRequestFeedback) {
		return respondStaleCallback(c)
	}
	current := h.StateManager.Get(c.Sender().ID)
	if !validFlow(current, "choosing_hotline_type", args[1]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	state := current
	state.Step = "awaiting_hotline_submission"
	state.HotlineType = args[0]
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	return editOrSend(
		c,
		hotlineTemplate(args[0]),
		hotlineCancelButton(state.FlowNonce),
	)
}

func (h *Handler) HandleCancelHotline(c tele.Context) error {
	args := callbackArguments(c)
	current := h.StateManager.Get(c.Sender().ID)
	if len(args) < 1 || !validFlow(current, "awaiting_hotline_submission", args[0]) {
		return respondStaleCallback(c)
	}
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль.")
	}
	if state == nil {
		state = &dto.UserState{}
	}
	state.Step = "choosing_hotline_type"
	state.FlowNonce = newFlowNonce()
	h.StateManager.Set(c.Sender().ID, state)
	return editOrSend(
		c,
		"Горячая линия\n\nВыберите тип обращения:",
		keyboards.HotlineTypeSelector(state.FlowNonce),
	)
}

func (h *Handler) HandleHotlineSubmission(c tele.Context, input string) error {
	state := h.StateManager.Get(c.Sender().ID)
	if state == nil || state.Step != "awaiting_hotline_submission" {
		return c.Send("Сначала откройте горячую линию: /hotline")
	}
	details := strings.TrimSpace(input)
	length := utf8.RuneCountInString(details)
	if length < 20 {
		return c.Send("Добавьте больше информации — минимум 20 символов.", hotlineCancelButton(state.FlowNonce))
	}
	if utf8.RuneCountInString(state.HotlineContext)+length > 4096 {
		return c.Send("Сообщение слишком длинное. Максимум — 4096 символов.", hotlineCancelButton(state.FlowNonce))
	}

	ctx, cancel := reqCtx()
	defer cancel()
	id, err := h.SupportRequestService.Submit(ctx, fmt.Sprint(c.Sender().ID), state.HotlineType, state.HotlineContext+details)
	if errors.Is(err, repository.ErrSupportRequestLimit) {
		return c.Send("У вас уже есть три открытых обращения. Дождитесь решения администратора.", hotlineCancelButton(state.FlowNonce))
	}
	if err != nil {
		slog.Error("submit hotline request failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось сохранить обращение. Попробуйте позже.", hotlineCancelButton(state.FlowNonce))
	}
	restored, _, restoreErr := h.restoreProfile(ctx, c.Sender().ID)
	if restoreErr != nil || restored == nil {
		h.StateManager.Delete(c.Sender().ID)
		return c.Send(fmt.Sprintf("Обращение принято. Номер заявки: %s\nОтвет придёт в этот чат.", id))
	}
	return c.Send(
		fmt.Sprintf("Обращение принято. Номер заявки: %s\nОтвет администратора придёт в этот чат.", id),
		keyboards.MainMenu(),
	)
}

func hotlineTemplate(requestType string) string {
	if requestType == domain.SupportRequestFeedback {
		return "Пожелания и обратная связь\n\nНапишите одним сообщением, что вы хотели бы улучшить или добавить. Можно рассказать о любой части бота — привязка к расписанию и шаблон не нужны. От 20 до 4096 символов.\n\nОбращение будет отправлено администраторам только после отправки вами текста."
	}
	if requestType == domain.SupportRequestNewInstitution {
		return "Скопируйте шаблон, заполните его и отправьте одним сообщением:\n\n" +
			"Учебное заведение:\n" +
			"Тип и город (вуз, колледж и т. п.):\n" +
			"Официальный сайт:\n" +
			"Прямая ссылка на расписание:\n" +
			"Как на сайте выбирается группа:\n" +
			"Дополнительный комментарий:"
	}
	return "Скопируйте шаблон, заполните его и отправьте одним сообщением:\n\n" +
		"Учебное заведение:\n" +
		"Группа или подразделение:\n" +
		"Ссылка на страницу расписания:\n" +
		"Что изменилось или работает неверно:\n" +
		"Дополнительный комментарий:"
}

func hotlineCancelButton(flowNonce string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Назад", "cancel_hotline", flowNonce)))
	return menu
}

func (h *Handler) HandleScheduleFeedback(c tele.Context) error {
	if isGroupChat(c) {
		return c.Respond(&tele.CallbackResponse{Text: "Отправьте обращение в личном чате с ботом"})
	}
	_ = c.Respond()
	args := callbackArguments(c)
	if len(args) != 2 {
		return respondStaleCallback(c)
	}
	date, err := time.Parse("2006-01-02", args[1])
	if err != nil {
		return respondStaleCallback(c)
	}
	ctx, cancel := reqCtx()
	defer cancel()
	target := h.scheduleCallbackTarget(ctx, c, args, 0)
	if target == nil {
		return nil
	}
	return h.openScheduleReport(c, target, date)
}
