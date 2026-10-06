package keyboards

import tele "gopkg.in/telebot.v3"

func ServiceUpdatesPrompt(key string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Да", "updates_choice", "on", key), menu.Data("Нет", "updates_choice", "off", key)))
	return menu
}

func ServiceUpdatesUnsubscribe() *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	menu.Inline(menu.Row(menu.Data("Отключить обновления сервиса", "updates_settings", "off")))
	return menu
}
