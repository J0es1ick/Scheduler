package handlers

import (
	"fmt"
	"log/slog"

	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleHelp(c tgbotapi.Context) error {
	isAdmin := false
	if sender := c.Sender(); sender != nil {
		ctx, cancel := reqCtx()
		defer cancel()

		var err error
		isAdmin, err = h.UserService.IsAdmin(ctx, fmt.Sprint(sender.ID))
		if err != nil {
			slog.Error(
				"help role check failed",
				"user_id", sender.ID,
				"err", err,
			)
		}
	}
	return c.Send(helpText(isAdmin))
}

func helpText(isAdmin bool) string {
	text := "Доступные команды:\n\n" +
		"/start — запустить бота\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/week — расписание на текущую неделю\n" +
		"/twoweeks — расписание на 2 недели\n" +
		"/change_group — добавить группу и сделать её основной\n" +
		"/change_university — сменить университет\n" +
		"/settings — подписки и уведомления\n" +
		"/hotline — сообщить о расписании или предложить новое учебное заведение\n" +
		"/privacy — узнать, какие данные хранит бот\n" +
		"/my_data — скачать копию своих данных\n" +
		"/delete_me — удалить профиль и связанные данные\n" +
		"/sources — информация об источниках расписания\n" +
		"/search — поиск занятий по критериям\n" +
		"/help — список команд"
	if isAdmin {
		text += "\n\nКоманды администратора:\n\n" +
			"/admin — открыть административную панель\n" +
			"/metrics — состояние источников, очередей и сервиса"
	}
	return text
}
