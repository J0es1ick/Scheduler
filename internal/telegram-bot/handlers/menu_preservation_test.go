package handlers

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func TestMainMenuKeepsPhotoAndUntrackedTextAfterRestart(t *testing.T) {
	for _, photo := range []bool{true, false} {
		scenario := newTelegramScenario(t)
		handler := &Handler{}
		message := &tele.Message{ID: 7, Text: "Сохранённое расписание", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, ReplyMarkup: keyboards.ScheduleDayNavigation(time.Now(), "4/147", false, "group")}
		if photo {
			message.Text = ""
			message.Photo = &tele.Photo{}
			message.Caption = "Сохранённая подпись"
		}
		c := scenario.bot.NewContext(tele.Update{Callback: &tele.Callback{ID: "menu", Sender: &tele.User{ID: 42}, Message: message}})
		if err := handler.leaveInlineForMenu(c); err != nil {
			t.Fatal(err)
		}
		if len(scenario.methods) != 1 || scenario.methods[0] != "editMessageReplyMarkup" {
			t.Fatalf("content was modified: %+v", scenario.methods)
		}
		if len(scenario.markup.InlineKeyboard) != 0 {
			t.Fatal("buttons remain")
		}
	}
}
