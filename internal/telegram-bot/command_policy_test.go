package bot

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestCommandStatePolicyInterruptsPrivateCommandsAndActions(t *testing.T) {
	manager := state.NewManager()
	telegramBot, err := tele.NewBot(tele.Settings{Offline: true, Synchronous: true})
	if err != nil {
		t.Fatal(err)
	}
	telegramBot.Me.Username = "scheduler_test_bot"
	telegramBot.Use(CommandStatePolicy(manager))
	telegramBot.Handle("/reminders", func(tele.Context) error { return nil })
	telegramBot.Handle("/help", func(tele.Context) error { return nil })
	telegramBot.Handle("Сегодня", func(tele.Context) error { return nil })
	telegramBot.Handle(tele.OnText, func(tele.Context) error { return nil })

	for _, text := range []string{
		"/reminders", "/quiet_hours", "/my_data", "/admin", "/metrics",
		"/help@scheduler_test_bot args", "/future_command", "Сегодня", " сегодня ", "ДВЕ НЕДЕЛИ",
	} {
		manager.Set(42, &dto.UserState{Step: "awaiting_query", FlowNonce: "flow"})
		telegramBot.ProcessUpdate(privateTextUpdate(text))
		if stored := manager.Get(42); stored != nil {
			t.Fatalf("%q preserved transient state: %+v", text, stored)
		}
	}
	manager.Set(42, &dto.UserState{Step: "done", PendingDeleteToken: "delete-token"})
	telegramBot.ProcessUpdate(privateTextUpdate("/metrics"))
	if stored := manager.Get(42); stored != nil {
		t.Fatalf("command preserved pending confirmation: %+v", stored)
	}
}

func TestCommandStatePolicyPreservesInputsCallbacksGroupsAndExplicitExceptions(t *testing.T) {
	manager := state.NewManager()
	telegramBot, err := tele.NewBot(tele.Settings{Offline: true, Synchronous: true})
	if err != nil {
		t.Fatal(err)
	}
	telegramBot.Use(CommandStatePolicy(manager, "keep_flow"))
	telegramBot.Handle("/keep_flow", func(tele.Context) error { return nil })
	telegramBot.Handle(tele.OnText, func(tele.Context) error { return nil })
	button := tele.Btn{Unique: "flow_callback"}
	telegramBot.Handle(&button, func(tele.Context) error { return nil })

	assertPreserved := func(name string, update tele.Update) {
		t.Helper()
		manager.Set(42, &dto.UserState{Step: "awaiting_query", FlowNonce: "flow"})
		telegramBot.ProcessUpdate(update)
		stored := manager.Get(42)
		if stored == nil || stored.Step != "awaiting_query" || stored.FlowNonce != "flow" {
			t.Fatalf("%s cleared transient state: %+v", name, stored)
		}
	}

	assertPreserved("dialog input", privateTextUpdate("4/147"))
	assertPreserved("explicit exception", privateTextUpdate("/keep_flow"))
	assertPreserved("group command", tele.Update{Message: &tele.Message{
		Text: "/unknown", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: -100, Type: tele.ChatGroup},
	}})
	assertPreserved("callback", tele.Update{Callback: &tele.Callback{
		ID: "callback", Sender: &tele.User{ID: 42}, Data: "\fflow_callback|flow",
		Message: &tele.Message{ID: 1, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}},
	}})
}

func privateTextUpdate(text string) tele.Update {
	return tele.Update{Message: &tele.Message{
		Text: text, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
	}}
}
