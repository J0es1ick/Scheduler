package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestSendScheduleMessageRemovesReplyKeyboardWithoutReencodingCallbacks(t *testing.T) {
	type request struct {
		method string
		body   map[string]string
	}
	var (
		mu       sync.Mutex
		requests []request
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode Telegram request: %v", err)
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		mu.Lock()
		requests = append(requests, request{method: method, body: body})
		requestNumber := len(requests)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if method == "deleteMessage" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":` +
			string(rune('0'+requestNumber)) +
			`,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL:         server.URL,
		Token:       "test-token",
		Client:      server.Client(),
		Offline:     true,
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	bot.Handle("/schedule", func(c tele.Context) error {
		date := time.Date(2026, time.September, 2, 0, 0, 0, 0, time.Local)
		return sendScheduleMessage(c, "Расписание", keyboards.ScheduleDayNavigation(date, "3/147", false, "group-id"))
	})

	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text:   "/schedule",
		Chat:   &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		Sender: &tele.User{ID: 42},
	}})

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("Telegram requests = %d, want 3", len(requests))
	}
	if requests[0].method != "sendMessage" || requests[1].method != "sendMessage" || requests[2].method != "deleteMessage" {
		t.Fatalf("unexpected Telegram request sequence: %s, %s, %s", requests[0].method, requests[1].method, requests[2].method)
	}

	var removeMarkup struct {
		RemoveKeyboard bool `json:"remove_keyboard"`
	}
	if err := json.Unmarshal([]byte(requests[0].body["reply_markup"]), &removeMarkup); err != nil {
		t.Fatalf("decode keyboard removal: %v", err)
	}
	if !removeMarkup.RemoveKeyboard {
		t.Fatal("first message must remove the persistent keyboard")
	}

	var navigation tele.ReplyMarkup
	if err := json.Unmarshal([]byte(requests[1].body["reply_markup"]), &navigation); err != nil {
		t.Fatalf("decode schedule navigation: %v", err)
	}
	callbackData := navigation.InlineKeyboard[0][0].Data
	if callbackData != "\fschedule_date|2026-09-01" {
		t.Fatalf("previous-day callback = %q", callbackData)
	}
}

func TestNormalizeCallbackArgumentsAcceptsPreviouslyDoubleEncodedButtons(t *testing.T) {
	args := normalizeCallbackArguments([]string{"\fschedule_date", "2026-09-02"})
	if len(args) != 1 || args[0] != "2026-09-02" {
		t.Fatalf("normalized callback arguments = %#v", args)
	}

	regular := normalizeCallbackArguments([]string{"2026-09-02"})
	if len(regular) != 1 || regular[0] != "2026-09-02" {
		t.Fatalf("regular callback arguments changed: %#v", regular)
	}
}

func TestCloseInlineRestoresPersistentMainMenuInPrivateChat(t *testing.T) {
	type request struct {
		method string
		body   map[string]string
	}
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode Telegram request: %v", err)
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		requests = append(requests, request{method: method, body: body})

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "sendMessage":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":8,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL:         server.URL,
		Token:       "test-token",
		Client:      server.Client(),
		Offline:     true,
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	handler := &Handler{}
	bot.Handle(&tele.Btn{Unique: "close_inline"}, handler.HandleCloseInline)
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID:     "callback-id",
		Data:   "\fclose_inline",
		Sender: &tele.User{ID: 42},
		Message: &tele.Message{
			ID:   7,
			Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		},
	}})

	if len(requests) != 3 {
		t.Fatalf("Telegram requests = %d, want 3", len(requests))
	}
	if requests[0].method != "answerCallbackQuery" || requests[1].method != "deleteMessage" || requests[2].method != "sendMessage" {
		t.Fatalf("unexpected close sequence: %s, %s, %s", requests[0].method, requests[1].method, requests[2].method)
	}
	var menu tele.ReplyMarkup
	if err := json.Unmarshal([]byte(requests[2].body["reply_markup"]), &menu); err != nil {
		t.Fatalf("decode restored main menu: %v", err)
	}
	if !menu.IsPersistent || len(menu.ReplyKeyboard) == 0 {
		t.Fatalf("close did not restore persistent main menu: %#v", menu)
	}
}

type navigationUserService struct {
	*service.UserService
	user domain.User
}

func (s *navigationUserService) GetUser(context.Context, string) (*domain.User, error) {
	user := s.user
	return &user, nil
}

type navigationGroupService struct {
	*service.GroupService
	group       domain.Group
	nameLookups int
}

func (s *navigationGroupService) GetGroupByID(context.Context, string) (*domain.Group, error) {
	group := s.group
	return &group, nil
}

func (s *navigationGroupService) GetGroupByName(context.Context, string, string) (*domain.Group, error) {
	s.nameLookups++
	return nil, nil
}

type navigationUniversityService struct {
	*service.UniversityService
	universities map[string]domain.University
}

func (s *navigationUniversityService) GetAll(context.Context) ([]domain.University, error) {
	result := make([]domain.University, 0, len(s.universities))
	for _, university := range s.universities {
		result = append(result, university)
	}
	return result, nil
}

func (s *navigationUniversityService) GetByID(_ context.Context, id string) (*domain.University, error) {
	university, ok := s.universities[id]
	if !ok {
		return nil, nil
	}
	return &university, nil
}

func TestUniversitySelectionBackAndCloseRestoreCompletedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		w.Header().Set("Content-Type", "application/json")
		if method == "answerCallbackQuery" || method == "deleteMessage" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":8,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL:         server.URL,
		Token:       "test-token",
		Client:      server.Client(),
		Offline:     true,
		Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}

	manager := state.NewManager()
	userService := &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "old-group"}}
	groupService := &navigationGroupService{group: domain.Group{
		ID: "old-group", UniversityID: "old-university", Name: "OLD-1", IsActive: true,
	}}
	universityService := &navigationUniversityService{universities: map[string]domain.University{
		"old-university": {ID: "old-university", Name: "Старый вуз", IsActive: true},
		"new-university": {ID: "new-university", Name: "Новый вуз", IsActive: true},
	}}
	handler := &Handler{
		StateManager:      manager,
		UserService:       userService,
		GroupService:      groupService,
		UniversityService: universityService,
	}
	manager.Set(42, &dto.UserState{
		UniversityID: "old-university", University: "Старый вуз", GroupID: "old-group", Query: "OLD-1", Step: "done",
	})

	bot.Handle(&tele.Btn{Unique: "select_university"}, handler.HandleUniversitySelect)
	bot.Handle(&tele.Btn{Unique: "back_university_selection"}, handler.HandleBackUniversitySelection)
	bot.Handle(&tele.Btn{Unique: "close_inline"}, handler.HandleCloseInline)
	bot.Handle(tele.OnText, handler.HandleTextInput)

	callback := func(userID int64, data string) {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
			ID: "callback-id", Data: data, Sender: &tele.User{ID: userID},
			Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: userID, Type: tele.ChatPrivate}},
		}})
	}

	callback(42, "\fselect_university|new-university")
	selected := manager.Get(42)
	if selected == nil || selected.Step != "awaiting_query" || selected.UniversityID != "new-university" {
		t.Fatalf("selected university state = %#v", selected)
	}

	callback(42, "\fback_university_selection")
	restored := manager.Get(42)
	if restored == nil || restored.Step != "done" || restored.GroupID != "old-group" {
		t.Fatalf("restored state = %#v", restored)
	}

	callback(42, "\fclose_inline")
	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "NEW-1", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
	}})

	if groupService.nameLookups != 0 {
		t.Fatalf("ordinary text triggered %d group lookups after navigation closed", groupService.nameLookups)
	}
	completed := manager.Get(42)
	if completed == nil || completed.Step != "done" || completed.GroupID != "old-group" {
		t.Fatalf("completed state after close = %#v", completed)
	}

	userService.user = domain.User{ID: "43"}
	manager.Set(43, &dto.UserState{
		UniversityID: "new-university", University: "Новый вуз", Step: "awaiting_query",
	})
	callback(43, "\fback_university_selection")
	if remaining := manager.Get(43); remaining != nil {
		t.Fatalf("state without a durable group = %#v", remaining)
	}
}
