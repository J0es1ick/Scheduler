package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
		return c.Send("Не удалось открыть горячую линию. Попробуйте позже.")
	}
	return c.Send(
		"Горячая линия расписаний\n\nЗдесь можно сообщить об изменениях на уже подключённом сайте или предложить новое учебное заведение. Выберите тип обращения:",
		keyboards.HotlineTypeSelector(),
	)
}

func (h *Handler) HandleHotlineType(c tele.Context) error {
	defer c.Respond()
	args := c.Args()
	if len(args) == 0 || (args[0] != domain.SupportRequestUpdateExisting && args[0] != domain.SupportRequestNewInstitution) {
		return c.Send("Не удалось определить тип обращения.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось загрузить профиль. Попробуйте позже.")
	}
	if state == nil {
		state = &dto.UserState{}
	}
	state.Step = "awaiting_hotline_submission"
	state.HotlineType = args[0]
	h.StateManager.Set(c.Sender().ID, state)
	_ = c.Edit("Тип обращения выбран.")
	return c.Send(hotlineTemplate(args[0]), hotlineCancelButton())
}

func (h *Handler) HandleCancelHotline(c tele.Context) error {
	defer c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось восстановить профиль.")
	}
	_ = c.Edit("Обращение отменено.")
	if state != nil {
		return c.Send("Главное меню:", keyboards.MainMenu())
	}
	return nil
}

func (h *Handler) HandleHotlineSubmission(c tele.Context, input string) error {
	state := h.StateManager.Get(c.Sender().ID)
	if state == nil || state.Step != "awaiting_hotline_submission" {
		return c.Send("Сначала откройте горячую линию: /hotline")
	}
	details := strings.TrimSpace(input)
	length := utf8.RuneCountInString(details)
	if length < 20 {
		return c.Send("Добавьте больше информации — минимум 20 символов.", hotlineCancelButton())
	}
	if length > 4096 {
		return c.Send("Сообщение слишком длинное. Максимум — 4096 символов.", hotlineCancelButton())
	}

	ctx, cancel := reqCtx()
	defer cancel()
	id, err := h.SupportRequestService.Submit(ctx, fmt.Sprint(c.Sender().ID), state.HotlineType, details)
	if errors.Is(err, repository.ErrSupportRequestLimit) {
		return c.Send("У вас уже есть три открытых обращения. Дождитесь решения администратора.")
	}
	if err != nil {
		slog.Error("submit hotline request failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось сохранить обращение. Попробуйте позже.")
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

func hotlineCancelButton() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Отменить обращение", "cancel_hotline")))
	return menu
}
