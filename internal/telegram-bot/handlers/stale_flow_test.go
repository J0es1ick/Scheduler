package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestStaleCancelCallbacksDoNotReplaceCurrentFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	manager := state.NewManager()
	handler := &Handler{StateManager: manager}
	bot.Handle(&tele.Btn{Unique: "cancel_search"}, handler.HandleCancelSearch)
	bot.Handle(&tele.Btn{Unique: "cancel_hotline"}, handler.HandleCancelHotline)
	bot.Handle(&tele.Btn{Unique: "close_inline"}, handler.HandleCloseInline)
	bot.Handle(&tele.Btn{Unique: "back_more"}, handler.HandleBackMore)
	bot.Handle(&tele.Btn{Unique: "cancel_search_type"}, handler.HandleCancelSearchType)
	bot.Handle(&tele.Btn{Unique: "cancel_hotline_type"}, handler.HandleCancelHotlineType)
	bot.Handle(&tele.Btn{Unique: "cancel_university_selection"}, handler.HandleCancelUniversitySelection)

	callback := func(data string) {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
			ID: "callback-id", Data: data, Sender: &tele.User{ID: 42},
			Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}},
		}})
	}
	manager.Set(42, &dto.UserState{Step: "awaiting_search_query", FlowNonce: "current-search"})
	callback("\fcancel_search|old-search")
	if current := manager.Get(42); current == nil || current.Step != "awaiting_search_query" || current.FlowNonce != "current-search" {
		t.Fatalf("stale search callback changed state: %#v", current)
	}

	manager.Set(42, &dto.UserState{Step: "awaiting_hotline_submission", FlowNonce: "current-hotline"})
	callback("\fcancel_hotline|old-hotline")
	if current := manager.Get(42); current == nil || current.Step != "awaiting_hotline_submission" || current.FlowNonce != "current-hotline" {
		t.Fatalf("stale hotline callback changed state: %#v", current)
	}

	for _, data := range []string{
		"\fclose_inline",
		"\fback_more",
		"\fcancel_search_type|old-search",
		"\fcancel_hotline_type|old-hotline",
		"\fcancel_university_selection|old-university",
	} {
		callback(data)
		if current := manager.Get(42); current == nil || current.Step != "awaiting_hotline_submission" || current.FlowNonce != "current-hotline" {
			t.Fatalf("stale generic callback %q changed state: %#v", data, current)
		}
	}
}

func TestStaticPagesFinishPreviousInputFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "answerCallbackQuery") {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()
	bot, err := tele.NewBot(tele.Settings{URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true})
	if err != nil {
		t.Fatal(err)
	}
	manager := state.NewManager()
	users := &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "g"}}
	groups := &navigationGroupService{group: domain.Group{ID: "g", UniversityID: "u", Name: "G", IsActive: true}}
	h := &Handler{StateManager: manager, UserService: users, GroupService: groups,
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "U", IsActive: true}}}}
	bot.Handle(&tele.Btn{Unique: "show_sources"}, h.HandleShowSources)
	bot.Handle(&tele.Btn{Unique: "show_help"}, h.HandleShowHelp)
	bot.Handle(&tele.Btn{Unique: "show_privacy"}, h.HandleShowPrivacy)
	bot.Handle(&tele.Btn{Unique: "show_connector"}, h.HandleShowConnector)
	bot.Handle("/sources", h.HandleSourcesInfo)
	bot.Handle("/help", h.HandleHelp)
	bot.Handle("/privacy", h.HandlePrivacy)
	bot.Handle("/connect_source", h.HandleConnectorInfo)
	bot.Handle(tele.OnText, h.HandleTextInput)
	pages := []struct {
		callback string
		command  string
	}{
		{callback: "show_sources", command: "/sources"},
		{callback: "show_help", command: "/help"},
		{callback: "show_privacy", command: "/privacy"},
		{callback: "show_connector", command: "/connect_source"},
	}
	for _, page := range pages {
		for _, direct := range []bool{false, true} {
			for _, withProfile := range []bool{true, false} {
				users.user.DefaultGroupID = ""
				if withProfile {
					users.user.DefaultGroupID = "g"
				}
				manager.Set(42, &dto.UserState{Step: "awaiting_query", UniversityID: "u", FlowNonce: "pending"})
				if direct {
					bot.ProcessUpdate(tele.Update{Message: &tele.Message{Text: page.command, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
				} else {
					bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{ID: "cb", Data: "\f" + page.callback, Sender: &tele.User{ID: 42}, Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}}})
				}
				current := manager.Get(42)
				if withProfile && (current == nil || current.Step != "done" || current.GroupID != "g") {
					t.Fatalf("%s retained input: %+v", page.command, current)
				}
				if !withProfile && current != nil {
					t.Fatalf("%s retained temporary profile: %+v", page.command, current)
				}
				bot.ProcessUpdate(tele.Update{Message: &tele.Message{Text: "random text", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
				if groups.nameLookups != 0 {
					t.Fatal("ordinary text changed group after static page")
				}
			}
		}
	}
}

func TestFlowCallbackPayloadsStayWithinTelegramLimit(t *testing.T) {
	for _, data := range []string{
		"\fconfirm_sub_delete|0123456789abcdef|99|0123456789abcdef",
		"\fcancel_sub_delete|0123456789abcdef|99|0123456789abcdef",
		"\fconfirm_unset_chat_group|0123456789abcdef",
		"\fcancel_unset_chat_group|0123456789abcdef",
		"\fcancel_university_selection|0123456789abcdef",
		"\fcancel_search_type|0123456789abcdef",
		"\fcancel_hotline_type|0123456789abcdef",
	} {
		if len([]byte(strings.TrimSpace(data))) > 64 {
			t.Fatalf("callback payload exceeds Telegram limit: %d bytes: %q", len([]byte(data)), data)
		}
	}
}
