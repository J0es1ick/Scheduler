package handlers

import tgbotapi "gopkg.in/telebot.v3"

func (h *Handler) HandleHelp(c tgbotapi.Context) error {
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
		"/search — поиск занятий по критериям\n" +
		"/help — список команд"
	return c.Send(text)
}
