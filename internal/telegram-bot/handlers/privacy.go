package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/miniapp"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandlePrivacy(c tele.Context) error {
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
	return c.Send(
		"Удалить профиль, подписки, ожидающие уведомления и обращения на горячую линию? Это действие нельзя отменить.",
		keyboards.DeleteProfileConfirmation(),
	)
}

func (h *Handler) HandleConfirmDeleteProfile(c tele.Context) error {
	_ = c.Respond()
	ctx, cancel := reqCtx()
	defer cancel()
	userID := fmt.Sprint(c.Sender().ID)
	if err := h.UserService.DeleteOwnData(ctx, userID); err != nil {
		slog.Error("delete own profile failed", "user_id", userID, "err", err)
		return c.Send("Не удалось удалить профиль. Если вы администратор, сначала снимите эту роль.")
	}
	h.StateManager.Delete(c.Sender().ID)
	if err := miniapp.ConfigureMenu(c.Bot(), c.Sender(), h.AdminPublicURL, false); err != nil {
		slog.Debug("reset menu after profile deletion failed", "user_id", userID, "err", err)
	}
	return c.Send("Профиль и связанные с ним данные удалены. Чтобы начать заново, используйте /start.")
}

func (h *Handler) HandleCancelDeleteProfile(c tele.Context) error {
	_ = c.Respond()
	return c.Send("Удаление отменено.")
}

func (h *Handler) HandleSourcesInfo(c tele.Context) error {
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
			"Политика данных: /privacy",
	)
	return text.String(), nil
}
