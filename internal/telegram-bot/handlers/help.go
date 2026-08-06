package handlers

import (
	"fmt"
	"log/slog"

	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleHelp(c tgbotapi.Context) error {
	if isGroupChat(c) {
		return c.Send(groupHelpText())
	}

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
	text := "Личное расписание:\n\n" +
		"/start — запустить бота\n" +
		"/menu — открыть компактное меню\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/date — выбрать произвольную дату\n" +
		"/week — расписание на текущую неделю\n" +
		"/twoweeks — расписание на 2 недели\n" +
		"/search — поиск занятий по критериям\n\n" +
		"Профиль и уведомления:\n\n" +
		"/change_group — добавить группу и сделать её основной\n" +
		"/change_university — сменить университет\n" +
		"/settings — подписки и уведомления\n" +
		"/subscriptions — открыть список подписок\n" +
		"/reminders — настроить напоминания перед занятиями\n" +
		"/my_data — скачать копию своих данных\n" +
		"/delete_me — удалить профиль и связанные данные\n\n" +
		"Информация и обратная связь:\n\n" +
		"/hotline — сообщить о расписании или предложить новое учебное заведение\n" +
		"/privacy — узнать, какие данные хранит бот\n" +
		"/sources — информация об источниках расписания\n" +
		"/help — список команд\n\n" +
		"Работа в групповых чатах:\n\n" +
		"/chat_settings — выбранная группа расписания чата\n" +
		"/set_chat_group isuct 3/147 — настроить группу чата\n" +
		"/unset_chat_group — удалить настройку чата\n\n" +
		"Последние две команды выполняются внутри группового чата " +
		"и доступны его администраторам."
	if isAdmin {
		text += "\n\nКоманды администратора сервиса:\n\n" +
			"/admin — открыть административную панель\n" +
			"/metrics — состояние источников, очередей и сервиса"
	}
	return text
}

func groupHelpText() string {
	return "Команды расписания этого чата:\n\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/date — выбрать произвольную дату\n" +
		"/week — расписание на текущую неделю\n" +
		"/twoweeks — расписание на две недели\n" +
		"/chat_settings — выбранная учебная группа\n" +
		"/sources — источники расписания\n\n" +
		"Только для администраторов чата:\n\n" +
		"/set_chat_group isuct 3/147 — выбрать группу\n" +
		"/unset_chat_group — удалить настройку"
}
