package bot

import (
	"testing"

	tele "gopkg.in/telebot.v3"
)

func TestConfigureIdentityAcceptsMentionedGroupCommand(t *testing.T) {
	telegramBot, err := tele.NewBot(tele.Settings{Offline: true, Synchronous: true})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}

	username, err := ConfigureIdentity(
		telegramBot,
		"@schedule_free_bot",
		"https://t.me/ignored_bot",
	)
	if err != nil {
		t.Fatalf("configure identity: %v", err)
	}
	if username != "schedule_free_bot" {
		t.Fatalf("username = %q", username)
	}

	handled := 0
	telegramBot.Handle("/sources", func(tele.Context) error {
		handled++
		return nil
	})

	telegramBot.ProcessUpdate(tele.Update{
		Message: &tele.Message{Text: "/sources@schedule_free_bot"},
	})
	if handled != 1 {
		t.Fatalf("mentioned command handled %d times, want 1", handled)
	}

	telegramBot.ProcessUpdate(tele.Update{
		Message: &tele.Message{Text: "/sources@another_schedule_bot"},
	})
	if handled != 1 {
		t.Fatalf("command for another bot must be ignored; handled %d times", handled)
	}
}

func TestConfigureIdentityFallsBackToPublicURL(t *testing.T) {
	telegramBot, err := tele.NewBot(tele.Settings{Offline: true})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}

	username, err := ConfigureIdentity(telegramBot, "", "https://t.me/schedule_free_bot")
	if err != nil {
		t.Fatalf("configure identity from public URL: %v", err)
	}
	if username != "schedule_free_bot" || telegramBot.Me.Username != username {
		t.Fatalf("configured username = %q, bot username = %q", username, telegramBot.Me.Username)
	}
}
