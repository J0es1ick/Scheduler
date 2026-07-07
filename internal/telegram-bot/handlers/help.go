package handlers

import tgbotapi "gopkg.in/telebot.v3"

func (h *Handler) HandleHelp(c tgbotapi.Context) error {
	text := "Доступные команды:\n\n" +
		"/start — запустить бота\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/week — расписание на текущую неделю\n" +
		"/twoweeks — расписание на 2 недели\n" +
		"/change_group — сменить группу\n" +
		"/change_university — сменить университет\n" +
		"/search — поиск занятий по критериям\n" +
		"/help — список команд"
	return c.Send(text)
}
