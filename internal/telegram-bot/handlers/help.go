package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"

	tgbotapi "gopkg.in/telebot.v3"
)

func (h *Handler) HandleHelp(c tgbotapi.Context) error {
	if isGroupChat(c) {
		return c.Send(groupHelpText())
	}
	if err := h.finishTransientFlow(c); err != nil {
		slog.Error("finish dialog before help failed", "user_id", c.Sender().ID, "err", err)
		return c.Send("Не удалось восстановить профиль. Попробуйте ещё раз позже.")
	}

	return c.Send(compactHelpText, helpCategories(h.helpAdmin(c)))
}

type helpTopic struct {
	id, title, text string
	admin           bool
}

var helpTopics = []helpTopic{
	{"schedule", "Расписание и экспорт", "/today — сегодня\n/tomorrow — завтра\n/date — календарь и произвольная дата\n/week — текущая неделя\n/twoweeks — две недели\n\nСтрелки листают дни и недели, «Выбрать дату» открывает календарь. В таблице видны окна между парами. «Скачать расписание»: PNG, JSON, CSV, ICS. ICS — разовый импорт, файл не обновляется автоматически. При возврате в главное меню расписание остаётся в чате без кнопок.", false},
	{"search", "Поиск и быстрые запросы", "/search — поиск группы, преподавателя, аудитории или дисциплины\n\nФамилия, фамилия с инициалами или «Поиск Конст» — найти преподавателя основного вуза. Несколько совпадений открывают выбор.\nСегодня, Завтра, Неделя, Две недели; Понедельник или Пн; дата 08.09.2026; число от -7 до 7 — смещение от сегодня.\nУ студента: 4/185, 4-185, 4 185 или «4 курс 185 группа» — выбрать основную группу; «ИГЭУ 1-ЭЭ-В» — сменить вуз и группу. У преподавателя такие запросы открывают расписание группы без изменения личной привязки.\nВ «Ещё» меняется формат результатов поиска: текст или таблица. Календарь и экспорт сохраняют открытое расписание и подгруппу.", false},
	{"settings", "Профиль и уведомления", "/change_group — сменить группу студента или своё ФИО\n/change_university — сменить вуз вместе с привязкой\n/settings или /subscriptions — настройки расписания\n/role — сменить роль, сохранив привязки\n/updates — сообщения об обновлениях сервиса; /updates on — включить, /updates off — отключить\n/daily — ежедневная отправка расписания; /daily 06:00 — время, /daily off — отключить\n/reminders — напоминания перед занятиями\n/quiet_hours — время без уведомлений\n\nУ студента: группы, основная группа, подгруппа и формат. У преподавателя: одно личное расписание по ФИО, его замена и формат. Подписки на изменения, общий переключатель уведомлений и личные напоминания настраиваются отдельно. Напоминания включаются по вашему выбору. Ежедневная отправка использует время вуза и выполняется даже во время тихих часов.", false},
	{"feedback", "Источники и обратная связь", "/hotline — ошибка расписания, новый вуз или свободное пожелание о работе бота\n/report — сообщить об ошибке; команду можно нажать под «Опубликовано». В обращении укажите вуз, группу или преподавателя и дату\n/sources — официальные источники и свежесть данных\n/connect_source — предложить парсер, JSON по HTTPS или внешний сервер\n\nОбращение отправляется после отправки вами текста, ответ администратора придёт в этот чат.", false},
	{"privacy", "Личные данные", "/privacy — какие данные хранит сервис\n/my_data — скачать копию своих данных\n/delete_me — удалить профиль и связанные данные после подтверждения", false},
	{"chats", "Групповые чаты и обмен", "/chat_settings — группа расписания чата и формат\n/set_chat_group <ID вуза> <название группы> — настроить группу чата\n/unset_chat_group — удалить привязку\n\nНастройки изменяют администраторы чата. Всем участникам доступны /today, /tomorrow, /date, /week, /twoweeks и /sources.\nВ любом чате: @имя_бота сегодня — поделиться своим расписанием через inline-поиск.", false},
	{"navigation", "Меню и начало работы", "/start — начать работу или открыть сохранённый профиль\n/menu — главное меню\n/help — разделы помощи и все команды\n\nПервый запуск: роль → вуз → группа или своё ФИО → подтверждение → предложение ежедневной отправки. Кнопка «Расписание» открывает личный редактор: преподаватель, аудитория, время и отмена пары на дату или до конца семестра. Правки действуют только для вас в боте, экспорте и автоматических сообщениях. Администратору сначала предлагается выбор сервиса.\n\nГлавное меню: периоды расписания, поиск, «Мои группы» или «Моё расписание» и «Ещё». «Ещё»: формат поиска, источники, подключение расписания, обратная связь, данные и помощь.", false},
	{"admin", "Команды администратора сервиса:", "/admin — административная панель\n/metrics — состояние источников, очередей и сервиса", true},
}

func helpText(isAdmin bool) string {
	var sections []string
	for _, topic := range helpTopics {
		if !topic.admin || isAdmin {
			sections = append(sections, topic.title+"\n\n"+topic.text)
		}
	}
	return strings.Join(sections, "\n\n")
}

func groupHelpText() string {
	return "Команды расписания этого чата:\n\n" +
		"/start — возможности бота и начало настройки чата\n" +
		"/today — расписание на сегодня\n" +
		"/tomorrow — расписание на завтра\n" +
		"/date — выбрать произвольную дату\n" +
		"/week — расписание на текущую неделю\n" +
		"/twoweeks — расписание на две недели\n" +
		"/chat_settings — выбранная учебная группа\n" +
		"/sources — источники расписания\n" +
		"/privacy — информация о данных и конфиденциальности\n" +
		"/help — помощь для группового чата\n\n" +
		"Кнопки расписания листают даты, открывают календарь, день, неделю или две недели. «Скачать расписание» — PNG, JSON, CSV и ICS (разовый импорт). В /chat_settings администратор выбирает формат: текст или таблица.\n\n" +
		"/connect_source — предложить парсер или другой способ подключения расписания\n\n" +
		"Только для администраторов чата:\n\n" +
		"/set_chat_group <ID вуза> <название группы> — выбрать группу\n" +
		"/unset_chat_group — удалить настройку\n\n" +
		"Личные подписки, напоминания, поиск, экспорт своих данных и горячая линия доступны в личном чате с ботом: /help."
}

const compactHelpText = "Помощь Scheduler\n\nВыберите раздел. Основную группу студента или единственную привязку преподавателя можно сменить в настройках; подписки на изменения и личные напоминания настраиваются отдельно."

func helpCategories(isAdmin ...bool) *tgbotapi.ReplyMarkup {
	menu := &tgbotapi.ReplyMarkup{}
	rows := []tgbotapi.Row{}
	for _, topic := range helpTopics {
		if topic.admin && (len(isAdmin) == 0 || !isAdmin[0]) {
			continue
		}
		rows = append(rows, menu.Row(menu.Data(topic.title, "help_topic", topic.id)))
	}
	rows = append(rows, menu.Row(menu.Data("Все команды", "help_topic", "all")), menu.Row(menu.Data("Назад", "back_more")))
	menu.Inline(rows...)
	return menu
}

func (h *Handler) helpAdmin(c tgbotapi.Context) bool {
	ctx, cancel := reqCtx()
	defer cancel()
	isAdmin, err := h.UserService.IsAdmin(ctx, fmt.Sprint(c.Sender().ID))
	return err == nil && isAdmin
}

func (h *Handler) HandleHelpTopic(c tgbotapi.Context) error {
	_ = c.Respond()
	if err := h.finishTransientFlow(c); err != nil {
		return c.Send("Не удалось восстановить профиль. Повторите /help позже.")
	}
	args := callbackArguments(c)
	if len(args) != 1 {
		return respondStaleCallback(c)
	}
	admin := h.helpAdmin(c)
	if args[0] == "all" {
		parts := service.SplitMessage(helpText(admin), tgMaxLen)
		if len(parts) == 1 {
			return editOrSend(c, parts[0], keyboards.BackToHelpMenu())
		}
		if err := c.Edit(&tgbotapi.ReplyMarkup{}); err != nil {
			return err
		}
		for i, part := range parts {
			var menu *tgbotapi.ReplyMarkup
			if i == len(parts)-1 {
				menu = keyboards.BackToHelpMenu()
			}
			if err := c.Send(part, menu); err != nil {
				return err
			}
		}
		return nil
	}
	for _, topic := range helpTopics {
		if topic.id == args[0] && (!topic.admin || admin) {
			return editOrSend(c, topic.title+"\n\n"+topic.text, keyboards.BackToHelpMenu())
		}
	}
	return respondStaleCallback(c)
}
