package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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
	handler := &Handler{}
	bot.Handle("/schedule", func(c tele.Context) error {
		date := time.Date(2026, time.September, 2, 0, 0, 0, 0, time.Local)
		return handler.sendScheduleMessage(c, "Расписание", keyboards.ScheduleDayNavigation(date, "3/147", false, "group-id"))
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
	if callbackData != "\fschedule_date|2026-09-01|"+keyboards.GroupToken("group-id") {
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

func TestScheduleChildMenuAndBackReuseMediaMessage(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		w.Header().Set("Content-Type", "application/json")
		if methods[len(methods)-1] == "answerCallbackQuery" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":0,"chat":{"id":42,"type":"private"},"caption":"ok","photo":[{"file_id":"photo","file_unique_id":"photo","width":1,"height":1}]}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	handler := &Handler{UniversityService: &navigationUniversityService{universities: map[string]domain.University{
		"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
	}}}
	bot.Handle(&tele.Btn{Unique: "open_child"}, func(c tele.Context) error {
		_ = c.Respond()
		return editScheduleOverlay(c, "Выберите формат файла:", keyboards.BackButton("back_schedule"))
	})
	bot.Handle(&tele.Btn{Unique: "back_schedule"}, func(c tele.Context) error {
		_ = c.Respond()
		date := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
		return handler.sendScheduleView(
			context.Background(), c, []dto.DaySchedule{{Date: date}},
			&scheduleTarget{UniversityID: "isuct", University: "ИГХТУ", GroupID: "group", GroupName: "4/147", ViewFormat: domain.ScheduleViewVisual},
			date, 1, keyboards.ScheduleDayNavigation(date, "4/147", false, "group"), "",
		)
	})

	callback := func(data string) {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
			ID: "callback-id", Data: data, Sender: &tele.User{ID: 42},
			Message: &tele.Message{
				ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
				Photo: &tele.Photo{File: tele.FromURL("https://example.test/schedule.png")},
			},
		}})
	}
	callback("\fopen_child")
	callback("\fback_schedule")

	want := []string{"answerCallbackQuery", "editMessageCaption", "answerCallbackQuery", "editMessageMedia"}
	if len(methods) != len(want) {
		t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
	}
	for index := range want {
		if methods[index] != want[index] {
			t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
		}
	}
}

func TestGenericMenuRemovesScheduleMediaBeforeSendingText(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		methods = append(methods, method)
		w.Header().Set("Content-Type", "application/json")
		if method == "sendMessage" {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":8,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	bot.Handle(&tele.Btn{Unique: "open_menu"}, func(c tele.Context) error {
		_ = c.Respond()
		return editOrSend(c, "Обычное меню", keyboards.BackButton("close_inline"))
	})
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\fopen_menu", Sender: &tele.User{ID: 42},
		Message: &tele.Message{
			ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
			Photo: &tele.Photo{File: tele.FromURL("https://example.test/schedule.png")},
		},
	}})

	want := []string{"answerCallbackQuery", "deleteMessage", "sendMessage"}
	if len(methods) != len(want) {
		t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
	}
	for index := range want {
		if methods[index] != want[index] {
			t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
		}
	}
}

func TestSplitCompactScheduleNavigationDeletesPreviousMessageSet(t *testing.T) {
	type request struct {
		method    string
		messageID string
	}
	var (
		requests []request
		nextID   = 1
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		requests = append(requests, request{method: method, messageID: body["message_id"]})
		w.Header().Set("Content-Type", "application/json")
		if method == "deleteMessage" || method == "answerCallbackQuery" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		messageID := nextID
		nextID++
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"ok":true,"result":{"message_id":%d,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`,
			messageID,
		)))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	handler := &Handler{UniversityService: &navigationUniversityService{universities: map[string]domain.University{
		"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
	}}}
	markup := keyboards.BackButton("refresh_schedule")
	bot.Handle("/multi", func(c tele.Context) error {
		return handler.replaceTrackedScheduleMessages(c, []string{"Первая часть", "Вторая часть"}, markup)
	})
	bot.Handle(&tele.Btn{Unique: "refresh_schedule"}, func(c tele.Context) error {
		_ = c.Respond()
		return handler.replaceTrackedScheduleMessages(c, []string{"Обновлённое расписание"}, markup)
	})
	bot.Handle(&tele.Btn{Unique: "open_main_menu"}, handler.HandleOpenMainMenu)
	bot.Handle(&tele.Btn{Unique: "empty_schedule"}, func(c tele.Context) error {
		_ = c.Respond()
		return handler.sendDaysWithMarkup(c, nil, "isuct", markup)
	})
	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "/multi", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
	}})
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\frefresh_schedule", Sender: &tele.User{ID: 42},
		Message: &tele.Message{ID: 3, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "Вторая часть"},
	}})

	var deleted []string
	for _, item := range requests {
		if item.method == "deleteMessage" {
			deleted = append(deleted, item.messageID)
		}
	}
	if !slices.Contains(deleted, "2") || !slices.Contains(deleted, "3") {
		t.Fatalf("deleted message ids = %#v, want both compact schedule parts", deleted)
	}

	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "/multi", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
	}})
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\fopen_main_menu", Sender: &tele.User{ID: 42},
		Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "Вторая часть"},
	}})

	deleted = deleted[:0]
	for _, item := range requests {
		if item.method == "deleteMessage" {
			deleted = append(deleted, item.messageID)
		}
	}
	if !slices.Contains(deleted, "6") || !slices.Contains(deleted, "7") {
		t.Fatalf("deleted message ids after main menu = %#v, want both compact schedule parts", deleted)
	}

	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "/multi", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
	}})
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\fempty_schedule", Sender: &tele.User{ID: 42},
		Message: &tele.Message{ID: 11, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "Вторая часть"},
	}})

	deleted = deleted[:0]
	for _, item := range requests {
		if item.method == "deleteMessage" {
			deleted = append(deleted, item.messageID)
		}
	}
	if !slices.Contains(deleted, "10") || !slices.Contains(deleted, "11") {
		t.Fatalf("deleted message ids for empty schedule = %#v, want both compact schedule parts", deleted)
	}
}

func TestSearchSelectionAndGroupChangeReuseTheirInlineMessage(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		methods = append(methods, method)
		w.Header().Set("Content-Type", "application/json")
		if method == "answerCallbackQuery" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	manager := state.NewManager()
	handler := &Handler{
		StateManager: manager,
		UserService:  &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "group"}},
		GroupService: &navigationGroupService{group: domain.Group{
			ID: "group", UniversityID: "isuct", Name: "4/147", IsActive: true,
		}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{
			"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
		}},
	}
	bot.Handle(&tele.Btn{Unique: "select_search_type"}, handler.HandleSearchTypeSelect)
	bot.Handle(&tele.Btn{Unique: "add_subscription"}, handler.HandleAddSubscription)

	manager.Set(42, &dto.UserState{
		UniversityID: "isuct", University: "ИГХТУ", GroupID: "group", Query: "4/147",
		Step: "choosing_search_type", FlowNonce: "search-flow", GroupActive: true,
	})
	callback := func(data string) {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
			ID: "callback-id", Data: data, Sender: &tele.User{ID: 42},
			Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "menu"},
		}})
	}
	callback("\fselect_search_type|group|search-flow")
	callback("\fadd_subscription|0")

	for _, method := range methods {
		if method == "sendMessage" {
			t.Fatalf("inline search/group-change flow created a new message: %#v", methods)
		}
	}
}

func TestHotlineTypeSelectionReusesInlineMessage(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		methods = append(methods, method)
		w.Header().Set("Content-Type", "application/json")
		if method == "answerCallbackQuery" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
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
	manager.Set(42, &dto.UserState{Step: "choosing_hotline_type", FlowNonce: "hotline-flow"})
	bot.Handle(&tele.Btn{Unique: "select_hotline_type"}, handler.HandleHotlineType)
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\fselect_hotline_type|new_institution|hotline-flow",
		Sender:  &tele.User{ID: 42},
		Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "menu"},
	}})

	for _, method := range methods {
		if method == "sendMessage" {
			t.Fatalf("hotline inline flow created a new message: %#v", methods)
		}
	}
}

func TestLegacyDetachedExportBackOnlyRemovesMenu(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
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
	handler := &Handler{}
	bot.Handle(&tele.Btn{Unique: "schedule_date"}, handler.HandleScheduleDateSelect)
	bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
		ID: "callback-id", Data: "\fschedule_date|2026-09-01", Sender: &tele.User{ID: 42},
		Message: &tele.Message{
			ID: 7, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Text: "Выберите формат файла:",
		},
	}})
	want := []string{"answerCallbackQuery", "deleteMessage"}
	if len(methods) != len(want) {
		t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
	}
	for index := range want {
		if methods[index] != want[index] {
			t.Fatalf("Telegram methods = %#v, want %#v", methods, want)
		}
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

func (s *navigationUserService) RegisterOrGetUser(context.Context, string, string) (*domain.User, error) {
	user := s.user
	return &user, nil
}

func (s *navigationUserService) GetUser(context.Context, string) (*domain.User, error) {
	user := s.user
	return &user, nil
}

func (s *navigationUserService) MarkTelegramMenuConfigured(context.Context, string, string) error {
	return nil
}

func (s *navigationUserService) IsAdmin(context.Context, string) (bool, error) {
	return s.user.IsAdmin, nil
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

func (s *navigationUniversityService) GetSourceFreshness(context.Context, string) (*domain.SourceFreshness, error) {
	return nil, nil
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
		UniversityID: "old-university", University: "Старый вуз", GroupID: "old-group", Query: "OLD-1",
		Step: "choosing_university", FlowNonce: "new-university-flow", GroupActive: true,
	})

	bot.Handle(&tele.Btn{Unique: "select_university"}, handler.HandleUniversitySelect)
	bot.Handle(&tele.Btn{Unique: "back_university_selection"}, handler.HandleBackUniversitySelection)
	bot.Handle(&tele.Btn{Unique: "cancel_university_selection"}, handler.HandleCancelUniversitySelection)
	bot.Handle(&tele.Btn{Unique: "close_inline"}, handler.HandleCloseInline)
	bot.Handle(tele.OnText, handler.HandleTextInput)

	callback := func(userID int64, data string) {
		bot.ProcessUpdate(tele.Update{Callback: &tele.Callback{
			ID: "callback-id", Data: data, Sender: &tele.User{ID: userID},
			Message: &tele.Message{ID: 7, Chat: &tele.Chat{ID: userID, Type: tele.ChatPrivate}},
		}})
	}

	callback(42, "\fselect_university|"+keyboards.UniversityToken("new-university")+"|new-university-flow")
	selected := manager.Get(42)
	if selected == nil || selected.Step != "awaiting_query" || selected.UniversityID != "new-university" {
		t.Fatalf("selected university state = %#v", selected)
	}

	manager.Set(44, &dto.UserState{
		Step: "choosing_university", FlowNonce: "legacy-university-flow",
	})
	callback(44, "\fselect_university|new-university|legacy-university-flow")
	legacySelected := manager.Get(44)
	if legacySelected == nil || legacySelected.Step != "awaiting_query" || legacySelected.UniversityID != "new-university" {
		t.Fatalf("legacy university callback state = %#v", legacySelected)
	}

	callback(42, "\fback_university_selection|"+selected.FlowNonce)
	restored := manager.Get(42)
	if restored == nil || restored.Step != "choosing_university" || restored.GroupID != "old-group" {
		t.Fatalf("restored state = %#v", restored)
	}

	callback(42, "\fcancel_university_selection|"+restored.FlowNonce)
	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "обычный текст", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
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
		FlowNonce: "new-user-flow",
	})
	callback(43, "\fback_university_selection|new-user-flow")
	newUserBack := manager.Get(43)
	if newUserBack == nil || newUserBack.Step != "choosing_university" || newUserBack.FlowNonce == "" {
		t.Fatalf("new-user back state = %#v", newUserBack)
	}
	callback(43, "\fcancel_university_selection|"+newUserBack.FlowNonce)
	if remaining := manager.Get(43); remaining != nil {
		t.Fatalf("state after cancelling new-user setup = %#v", remaining)
	}
}

func TestStartDatePayloadOpensCalendar(t *testing.T) {
	var sentTexts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if method == "sendMessage" {
			sentTexts = append(sentTexts, body["text"])
		}
		w.Header().Set("Content-Type", "application/json")
		if method == "setChatMenuButton" {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":8,"date":0,"chat":{"id":42,"type":"private"},"text":"ok"}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatalf("create offline bot: %v", err)
	}
	handler := &Handler{
		StateManager: state.NewManager(),
		UserService: &navigationUserService{user: domain.User{
			ID: "42", DefaultGroupID: "group",
		}},
		GroupService: &navigationGroupService{group: domain.Group{
			ID: "group", UniversityID: "isuct", Name: "4/147", IsActive: true,
		}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{
			"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true, Timezone: "Europe/Moscow"},
		}},
	}
	bot.Handle("/start", handler.HandleStart)
	bot.ProcessUpdate(tele.Update{Message: &tele.Message{
		Text: "/start date", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}, Sender: &tele.User{ID: 42},
	}})
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Выберите дату") {
		t.Fatalf("/start date sent texts = %#v", sentTexts)
	}
}
