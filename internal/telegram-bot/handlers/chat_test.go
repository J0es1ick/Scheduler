package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type unlinkChatService struct {
	*service.ChatProfileService
	deleted int
}

func (s *unlinkChatService) Delete(context.Context, string) error { s.deleted++; return nil }

func TestUnsetChatCommandRequiresConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "getChatMember"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"user":{"id":42},"status":"administrator"}}`))
		case strings.HasSuffix(r.URL.Path, "answerCallbackQuery"):
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":-1,"type":"supergroup"},"text":"ok"}}`))
		}
	}))
	defer server.Close()
	bot, err := tele.NewBot(tele.Settings{URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true})
	if err != nil {
		t.Fatal(err)
	}
	profiles := &unlinkChatService{}
	manager := state.NewManager()
	h := &Handler{ChatProfileService: profiles, StateManager: manager}
	bot.Handle("/unset_chat_group", h.HandleUnsetChatGroup)
	bot.Handle(&tele.Btn{Unique: "confirm_unset_chat_group"}, h.HandleConfirmUnsetChatGroup)
	bot.ProcessUpdate(tele.Update{Message: &tele.Message{Text: "/unset_chat_group", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: -1, Type: tele.ChatSuperGroup}}})
	current := manager.Get(42)
	if profiles.deleted != 0 || current == nil || current.PendingChatUnlinkToken == "" {
		t.Fatal("command deleted profile without confirmation")
	}
	data := "\fconfirm_unset_chat_group|" + current.PendingChatUnlinkToken
	for range 2 {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{ID: "cb", Data: data, Sender: &tele.User{ID: 42}, Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: -1, Type: tele.ChatSuperGroup}}}})
	}
	if profiles.deleted != 1 {
		t.Fatalf("deleted %d times", profiles.deleted)
	}
}

func TestChatGroupArguments(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantUniversity string
		wantGroup      string
		wantOK         bool
	}{
		{
			name:           "university and group",
			args:           []string{"ISUCT", "3/147"},
			wantUniversity: "isuct",
			wantGroup:      "3/147",
			wantOK:         true,
		},
		{
			name:           "colon notation",
			args:           []string{"ispu:1-40"},
			wantUniversity: "ispu",
			wantGroup:      "1-40",
			wantOK:         true,
		},
		{
			name:      "group only",
			args:      []string{"3/147"},
			wantGroup: "3/147",
			wantOK:    true,
		},
		{name: "empty", args: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			university, group, ok := chatGroupArguments(test.args)
			if university != test.wantUniversity ||
				group != test.wantGroup ||
				ok != test.wantOK {
				t.Errorf(
					"chatGroupArguments() = (%q, %q, %v), want (%q, %q, %v)",
					university,
					group,
					ok,
					test.wantUniversity,
					test.wantGroup,
					test.wantOK,
				)
			}
		})
	}
}
