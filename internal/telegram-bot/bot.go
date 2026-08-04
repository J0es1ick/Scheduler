package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	tele "gopkg.in/telebot.v3"
)

func Register(ctx context.Context, bot *tele.Bot, handler *handlers.Handler) <-chan struct{} {
	privateCommands := []tele.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "help", Description: "Список команд"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "date", Description: "Расписание на выбранную дату"},
		{Text: "week", Description: "Расписание на неделю"},
		{Text: "twoweeks", Description: "Расписание на две недели"},
		{Text: "search", Description: "Поиск занятий"},
		{Text: "change_group", Description: "Добавить или сменить основную группу"},
		{Text: "change_university", Description: "Выбрать другой вуз"},
		{Text: "settings", Description: "Подписки и уведомления"},
		{Text: "subscriptions", Description: "Подписки на группы"},
		{Text: "reminders", Description: "Напоминания перед занятиями"},
		{Text: "hotline", Description: "Горячая линия расписаний"},
		{Text: "privacy", Description: "Какие данные хранит бот"},
		{Text: "my_data", Description: "Скачать мои данные"},
		{Text: "delete_me", Description: "Удалить мой профиль"},
		{Text: "sources", Description: "Источники расписания"},
	}
	groupCommands := []tele.Command{
		{Text: "help", Description: "Список команд"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "date", Description: "Расписание на выбранную дату"},
		{Text: "week", Description: "Расписание на неделю"},
		{Text: "twoweeks", Description: "Расписание на две недели"},
		{Text: "chat_settings", Description: "Группа расписания этого чата"},
		{Text: "sources", Description: "Источники расписания"},
	}
	groupAdminCommands := append(
		append([]tele.Command{}, groupCommands...),
		tele.Command{Text: "set_chat_group", Description: "Выбрать группу расписания"},
		tele.Command{Text: "unset_chat_group", Description: "Удалить группу расписания"},
	)
	commandsReady := make(chan struct{})
	go configureCommands(ctx, bot, privateCommands, groupCommands, groupAdminCommands, commandsReady)

	bot.Handle("/start", handler.HandleStart)
	bot.Handle("/help", handler.HandleHelp)
	bot.Handle("/today", handler.HandleToday)
	bot.Handle("/tomorrow", handler.HandleTomorrow)
	bot.Handle("/date", handler.HandleDate)
	bot.Handle("/week", handler.HandleWeek)
	bot.Handle("/twoweeks", handler.HandleTwoWeeks)
	bot.Handle("/search", handler.PrivateOnly(handler.HandleSearch))
	bot.Handle("/change_group", handler.PrivateOnly(handler.HandleChangeGroup))
	bot.Handle("/change_university", handler.PrivateOnly(handler.HandleChangeUniversity))
	bot.Handle("/settings", handler.PrivateOnly(handler.HandleSettings))
	bot.Handle("/subscriptions", handler.PrivateOnly(handler.HandleSettings))
	bot.Handle("/reminders", handler.PrivateOnly(handler.HandleReminders))
	bot.Handle("/hotline", handler.PrivateOnly(handler.HandleHotline))
	bot.Handle("/admin", handler.PrivateOnly(handler.HandleAdmin))
	bot.Handle("/privacy", handler.HandlePrivacy)
	bot.Handle("/my_data", handler.PrivateOnly(handler.HandleMyData))
	bot.Handle("/delete_me", handler.PrivateOnly(handler.HandleDeleteMe))
	bot.Handle("/sources", handler.HandleSourcesInfo)
	bot.Handle("/metrics", handler.PrivateOnly(handler.HandleMetrics))
	bot.Handle("/chat_settings", handler.HandleChatSettings)
	bot.Handle("/set_chat_group", handler.HandleSetChatGroup)
	bot.Handle("/unset_chat_group", handler.HandleUnsetChatGroup)

	bot.Handle(&tele.Btn{Unique: "select_university"}, handler.HandleUniversitySelect)
	bot.Handle(&tele.Btn{Unique: "select_search_type"}, handler.HandleSearchTypeSelect)
	bot.Handle(&tele.Btn{Unique: "select_weekday"}, handler.HandleWeekDaySelect)
	bot.Handle(&tele.Btn{Unique: "schedule_date"}, handler.HandleScheduleDateSelect)
	bot.Handle(&tele.Btn{Unique: "calendar_month"}, handler.HandleCalendarMonth)
	bot.Handle(&tele.Btn{Unique: "calendar_noop"}, handler.HandleCalendarNoop)
	bot.Handle(&tele.Btn{Unique: "cancel_search"}, handler.HandleCancelSearch)
	bot.Handle(&tele.Btn{Unique: "set_default_subscription"}, handler.HandleSetDefaultSubscription)
	bot.Handle(&tele.Btn{Unique: "delete_subscription"}, handler.HandleDeleteSubscription)
	bot.Handle(&tele.Btn{Unique: "toggle_notifications"}, handler.HandleToggleNotifications)
	bot.Handle(&tele.Btn{Unique: "show_reminder_settings"}, handler.HandleShowReminderSettings)
	bot.Handle(&tele.Btn{Unique: "set_reminder"}, handler.HandleSetReminder)
	bot.Handle(&tele.Btn{Unique: "back_subscription_settings"}, handler.HandleBackSubscriptionSettings)
	bot.Handle(&tele.Btn{Unique: "select_hotline_type"}, handler.HandleHotlineType)
	bot.Handle(&tele.Btn{Unique: "cancel_hotline"}, handler.HandleCancelHotline)
	bot.Handle(&tele.Btn{Unique: "confirm_delete_profile"}, handler.HandleConfirmDeleteProfile)
	bot.Handle(&tele.Btn{Unique: "cancel_delete_profile"}, handler.HandleCancelDeleteProfile)

	bot.Handle("На сегодня", handler.HandleToday)
	bot.Handle("На завтра", handler.HandleTomorrow)
	bot.Handle("На неделю", handler.HandleWeek)
	bot.Handle("По дню недели", handler.HandleWeekDay)
	bot.Handle("Поиск", handler.PrivateOnly(handler.HandleSearch))
	bot.Handle("Сменить группу", handler.PrivateOnly(handler.HandleChange))
	bot.Handle("Добавить группу", handler.PrivateOnly(handler.HandleChange))
	bot.Handle("Настройки", handler.PrivateOnly(handler.HandleSettings))
	bot.Handle("Горячая линия", handler.PrivateOnly(handler.HandleHotline))

	bot.Handle(tele.OnText, handler.HandleTextInput)
	return commandsReady
}

func configureCommands(
	ctx context.Context,
	bot *tele.Bot,
	privateCommands []tele.Command,
	groupCommands []tele.Command,
	groupAdminCommands []tele.Command,
	ready chan<- struct{},
) {
	for {
		err := bot.SetCommands(privateCommands)
		if err == nil {
			err = bot.SetCommands(
				privateCommands,
				tele.CommandScope{Type: tele.CommandScopeAllPrivateChats},
			)
		}
		if err == nil {
			err = bot.SetCommands(
				groupCommands,
				tele.CommandScope{Type: tele.CommandScopeAllGroupChats},
			)
		}
		if err == nil {
			err = bot.SetCommands(
				groupAdminCommands,
				tele.CommandScope{Type: tele.CommandScopeAllChatAdmin},
			)
		}
		if err == nil {
			slog.Info("Telegram commands configured")
			close(ready)
			return
		}
		slog.Warn("Telegram commands configuration failed; retrying", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
	}
}
