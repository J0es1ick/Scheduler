package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandlePrivacy(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		slog.Error("finish dialog before privacy failed", "err", err)
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	return c.Send(privacyText())
}

func privacyText() string {
	return "Какие данные хранит бот:\n\n" +
		"• Telegram ID и имя пользователя — для профиля и доставки сообщений;\n" +
		"• выбранные группы и настройку уведомлений;\n" +
		"• обращения об источниках расписания и их статус;\n" +
		"• технические события доставки без содержимого личной переписки.\n\n" +
		"Бот не получает номер телефона и не читает другие чаты. " +
		"Получить копию данных: /my_data\nУдалить профиль: /delete_me"
}

func (h *Handler) HandleMyData(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	export, err := h.UserService.ExportData(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil {
		slog.Error("user data export failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Профиль не найден или данные временно недоступны. Запустите /start и попробуйте снова.")
	}
	payload, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return c.Send("Не удалось подготовить файл с данными.")
	}
	document := &tele.Document{
		File:     tele.FromReader(bytes.NewReader(payload)),
		FileName: "scheduler-my-data.json",
		Caption:  "Копия данных, которые Scheduler хранит о вашем профиле.",
	}
	return c.Send(document)
}

func (h *Handler) HandleDeleteMe(c tele.Context) error {
	ctx, cancel := reqCtx()
	defer cancel()
	user, err := h.UserService.GetUser(ctx, fmt.Sprint(c.Sender().ID))
	if err != nil || user == nil {
		return c.Send("Профиль не найден.")
	}
	if user.IsAdmin {
		return c.Send("Сначала снимите с профиля роль администратора. Это защищает сервис от случайной потери последнего доступа.")
	}
	state, _, err := h.restoreProfile(ctx, c.Sender().ID)
	if err != nil {
		return c.Send("Не удалось подготовить удаление профиля. Попробуйте позже.")
	}
	if state == nil {
		state = &dto.UserState{Step: "done"}
	}
	state.PendingDeleteToken = newFlowNonce()
	state.PendingDeleteExpiresAt = time.Now().Add(10 * time.Minute)
	h.StateManager.Set(c.Sender().ID, state)
	return c.Send(
		"Удалить профиль, подписки, ожидающие уведомления и обращения на горячую линию? Это действие нельзя отменить.",
		keyboards.DeleteProfileConfirmation(state.PendingDeleteToken),
	)
}

func (h *Handler) HandleConfirmDeleteProfile(c tele.Context) error {
	token, ok := callbackArgument(c)
	state := h.StateManager.Get(c.Sender().ID)
	if !ok || !consumeDeleteIntent(state, token, time.Now()) {
		return c.Respond(&tele.CallbackResponse{Text: "Подтверждение устарело. Запустите /delete_me снова.", ShowAlert: true})
	}
	h.StateManager.Set(c.Sender().ID, state)
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	if err := h.UserService.RequestOwnDataDeletion(ctx, userID); err != nil {
		slog.Error("request own profile deletion failed", "user_id", userID, "err", err)
		return c.Send("Не удалось принять запрос на удаление. Если вы администратор, сначала снимите эту роль.")
	}
	h.StateManager.Delete(c.Sender().ID)
	if err := h.configureMiniAppMenu(ctx, c.Bot(), c.Sender(), false); err != nil {
		slog.Debug("reset menu after profile deletion failed", "user_id", userID, "err", err)
	}
	return c.Send("Запрос на удаление принят. Профиль и связанные данные будут удалены в ближайшее время.")
}

func (h *Handler) HandleCancelDeleteProfile(c tele.Context) error {
	token, ok := callbackArgument(c)
	state := h.StateManager.Get(c.Sender().ID)
	if !ok || !consumeDeleteIntent(state, token, time.Now()) {
		return c.Respond(&tele.CallbackResponse{Text: "Подтверждение уже недействительно"})
	}
	h.StateManager.Set(c.Sender().ID, state)
	_ = c.Respond()
	_ = c.Edit("Удаление отменено.")
	return h.HandleMenu(c)
}

func consumeDeleteIntent(state *dto.UserState, token string, now time.Time) bool {
	if state == nil || token == "" || state.PendingDeleteToken != token || !state.PendingDeleteExpiresAt.After(now) {
		return false
	}
	state.PendingDeleteToken = ""
	state.PendingDeleteExpiresAt = time.Time{}
	return true
}

func (h *Handler) HandleSourcesInfo(c tele.Context) error {
	if err := h.finishTransientFlow(c); err != nil {
		slog.Error("finish dialog before sources failed", "err", err)
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}
	ctx, cancel := reqCtx()
	defer cancel()
	text, err := h.sourcesInfoText(ctx)
	if err != nil {
		slog.Error("load public schedule sources failed", "err", err)
		return c.Send("Не удалось загрузить список источников. Попробуйте позже.")
	}
	return c.Send(text)
}

func (h *Handler) sourcesInfoText(ctx context.Context) (string, error) {
	universities, err := h.UniversityService.GetAll(ctx)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	text.WriteString("Источники расписания\n\n")
	for _, university := range universities {
		if !university.IsActive {
			continue
		}
		text.WriteString("• ")
		text.WriteString(university.Name)
		if university.ScheduleURL != "" {
			text.WriteString(" — ")
			text.WriteString(university.ScheduleURL)
		}
		text.WriteByte('\n')
	}
	text.WriteString(
		"\nДанные проходят автоматическую проверку перед публикацией. " +
			"Сомнительные обновления отправляются администратору на проверку.\n\n" +
			"Предложить новый источник или исправление: «Ещё» → «Сообщить о расписании» или /hotline\n" +
			"Для разработчиков парсеров: /connect_source\n" +
			"Политика данных: /privacy",
	)
	return text.String(), nil
}
