package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/handlers"
	tele "gopkg.in/telebot.v3"
)

func privateCommands() []tele.Command {
	return []tele.Command{
		{Text: "start", Description: "Запустить бота"},
		{Text: "menu", Description: "Открыть меню"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "week", Description: "Расписание на неделю"},
		{Text: "date", Description: "Выбрать дату"},
		{Text: "search", Description: "Поиск занятий"},
		{Text: "settings", Description: "Мои группы и уведомления"},
		{Text: "help", Description: "Помощь и остальные команды"},
	}
}

func adminPrivateCommands() []tele.Command {
	commands := append([]tele.Command{}, privateCommands()...)
	return append(commands,
		tele.Command{Text: "admin", Description: "Открыть админ-панель"},
		tele.Command{Text: "metrics", Description: "Состояние сервиса"},
	)
}

func groupCommands() []tele.Command {
	return []tele.Command{
		{Text: "help", Description: "Список команд"},
		{Text: "today", Description: "Расписание на сегодня"},
		{Text: "tomorrow", Description: "Расписание на завтра"},
		{Text: "date", Description: "Расписание на выбранную дату"},
		{Text: "week", Description: "Расписание на неделю"},
		{Text: "chat_settings", Description: "Группа расписания этого чата"},
		{Text: "sources", Description: "Источники расписания"},
	}
}

func groupAdminCommands() []tele.Command {
	return append(
		append([]tele.Command{}, groupCommands()...),
		tele.Command{Text: "set_chat_group", Description: "Выбрать группу расписания"},
		tele.Command{Text: "unset_chat_group", Description: "Удалить группу расписания"},
	)
}

func Register(ctx context.Context, bot *tele.Bot, handler *handlers.Handler) <-chan struct{} {
	commandsReady := make(chan struct{})
	go configureCommands(ctx, bot, privateCommands(), groupCommands(), groupAdminCommands(), commandsReady)

	bot.Handle("/start", refreshPrivateCommandScope(bot, handler, handler.HandleStart))
	bot.Handle("/menu", handler.PrivateOnly(handler.HandleMenu))
	bot.Handle("/help", refreshPrivateCommandScope(bot, handler, handler.HandleHelp))
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
	bot.Handle(&tele.Btn{Unique: "open_calendar"}, handler.HandleOpenCalendar)
	bot.Handle(&tele.Btn{Unique: "calendar_noop"}, handler.HandleCalendarNoop)
	bot.Handle(&tele.Btn{Unique: "schedule_week"}, handler.HandleScheduleWeekSelect)
	bot.Handle(&tele.Btn{Unique: "open_weekday"}, handler.HandleOpenWeekday)
	bot.Handle(&tele.Btn{Unique: "open_schedule_group"}, handler.HandleOpenScheduleGroup)
	bot.Handle(&tele.Btn{Unique: "open_main_menu"}, handler.HandleOpenMainMenu)
	bot.Handle(&tele.Btn{Unique: "cancel_search"}, handler.HandleCancelSearch)
	bot.Handle(&tele.Btn{Unique: "close_inline"}, handler.HandleCloseInline)
	bot.Handle(&tele.Btn{Unique: "back_more"}, handler.HandleBackMore)
	bot.Handle(&tele.Btn{Unique: "subscription_page"}, handler.HandleSubscriptionPage)
	bot.Handle(&tele.Btn{Unique: "open_subscription"}, handler.HandleOpenSubscription)
	bot.Handle(&tele.Btn{Unique: "add_subscription"}, handler.HandleAddSubscription)
	bot.Handle(&tele.Btn{Unique: "set_default_subscription"}, handler.HandleSetDefaultSubscription)
	bot.Handle(&tele.Btn{Unique: "delete_subscription"}, handler.HandleDeleteSubscription)
	bot.Handle(&tele.Btn{Unique: "request_delete_subscription"}, handler.HandleRequestDeleteSubscription)
	bot.Handle(&tele.Btn{Unique: "confirm_delete_subscription"}, handler.HandleConfirmDeleteSubscription)
	bot.Handle(&tele.Btn{Unique: "toggle_notifications"}, handler.HandleToggleNotifications)
	bot.Handle(&tele.Btn{Unique: "show_reminder_settings"}, handler.HandleShowReminderSettings)
	bot.Handle(&tele.Btn{Unique: "set_reminder"}, handler.HandleSetReminder)
	bot.Handle(&tele.Btn{Unique: "back_subscription_settings"}, handler.HandleBackSubscriptionSettings)
	bot.Handle(&tele.Btn{Unique: "select_hotline_type"}, handler.HandleHotlineType)
	bot.Handle(&tele.Btn{Unique: "cancel_hotline"}, handler.HandleCancelHotline)
	bot.Handle(&tele.Btn{Unique: "show_sources"}, handler.HandleShowSources)
	bot.Handle(&tele.Btn{Unique: "open_hotline"}, handler.HandleOpenHotline)
	bot.Handle(&tele.Btn{Unique: "show_privacy"}, handler.HandleShowPrivacy)
	bot.Handle(&tele.Btn{Unique: "show_help"}, handler.HandleShowHelp)
	bot.Handle(&tele.Btn{Unique: "chat_change_group"}, handler.HandleChatChangeGroup)
	bot.Handle(&tele.Btn{Unique: "request_unset_chat_group"}, handler.HandleRequestUnsetChatGroup)
	bot.Handle(&tele.Btn{Unique: "confirm_unset_chat_group"}, handler.HandleConfirmUnsetChatGroup)
	bot.Handle(&tele.Btn{Unique: "confirm_delete_profile"}, handler.HandleConfirmDeleteProfile)
	bot.Handle(&tele.Btn{Unique: "cancel_delete_profile"}, handler.HandleCancelDeleteProfile)

	bot.Handle("Сегодня", handler.HandleToday)
	bot.Handle("Завтра", handler.HandleTomorrow)
	bot.Handle("Неделя", handler.HandleWeek)
	bot.Handle("Выбрать дату", handler.HandleDate)
	bot.Handle("По дню недели", handler.HandleWeekDay)
	bot.Handle("Поиск", handler.PrivateOnly(handler.HandleSearch))
	bot.Handle("Мои группы", handler.PrivateOnly(handler.HandleSettings))
	bot.Handle("Ещё", handler.PrivateOnly(handler.HandleMore))
	// Old reply-keyboard labels remain supported while Telegram clients replace
	// their cached keyboard with the compact one.
	bot.Handle("На сегодня", handler.HandleToday)
	bot.Handle("На завтра", handler.HandleTomorrow)
	bot.Handle("На неделю", handler.HandleWeek)
	bot.Handle("Сменить группу", handler.PrivateOnly(handler.HandleChange))
	bot.Handle("Добавить группу", handler.PrivateOnly(handler.HandleChange))
	bot.Handle("Настройки", handler.PrivateOnly(handler.HandleSettings))
	bot.Handle("Горячая линия", handler.PrivateOnly(handler.HandleHotline))

	bot.Handle(tele.OnText, handler.HandleTextInput)
	return commandsReady
}

func refreshPrivateCommandScope(
	bot *tele.Bot,
	handler *handlers.Handler,
	next tele.HandlerFunc,
) tele.HandlerFunc {
	return func(c tele.Context) error {
		if err := next(c); err != nil {
			return err
		}
		if c.Chat() == nil || c.Chat().Type != tele.ChatPrivate || c.Sender() == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		isAdmin, err := handler.UserService.IsAdmin(ctx, fmt.Sprint(c.Sender().ID))
		if err != nil {
			slog.Debug("private command scope role check failed", "user_id", c.Sender().ID, "err", err)
			return nil
		}
		commands := privateCommands()
		if isAdmin {
			commands = adminPrivateCommands()
		}
		if err = bot.SetCommands(commands, tele.CommandScope{
			Type:   tele.CommandScopeChat,
			ChatID: c.Chat().ID,
		}); err != nil {
			slog.Debug("private command scope update failed", "user_id", c.Sender().ID, "err", err)
		}
		return nil
	}
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
