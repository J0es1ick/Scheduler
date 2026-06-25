package handlers

import tele "gopkg.in/telebot.v3"

func HandleHelp(c tele.Context) error {
	text := "Доступные команды:\n\n" +
		"/start — запустить бота\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/week — расписание на текущую неделю\n" +
		"/change_university — сменить университет\n" +
		"/help — список команд"
	return c.Send(text)
}
