package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
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
		return sendScheduleMessage(c, "Расписание", keyboards.ScheduleDayNavigation(date, "3/147", false))
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
