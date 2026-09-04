package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

func TestLongScheduleReplacementWithProductionLimiter(t *testing.T) {
	var mu sync.Mutex
	sent := 0
	deleted := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/deleteMessage") {
			if sent != 8 {
				t.Errorf("old schedule deleted before all new parts arrived: sent=%d", sent)
			}
			deleted = append(deleted, body["message_id"])
			fmt.Fprint(w, `{"ok":true,"result":true}`)
			return
		}
		sent++
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"chat":{"id":42,"type":"private"}}}`, 100+sent)
	}))
	defer server.Close()
	limiter := telegramlimit.New(telegramlimit.DefaultGlobalInterval, telegramlimit.DefaultRecipientInterval)
	client := server.Client()
	client.Transport = telegramlimit.NewTransport(client.Transport, limiter)
	bot, err := tele.NewBot(tele.Settings{URL: server.URL, Token: "test-token", Client: client, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	current := bot.NewContext(tele.Update{Callback: &tele.Callback{
		ID: "long-schedule", Sender: &tele.User{ID: 42},
		Message: &tele.Message{ID: 8, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}},
	}})
	handler := &Handler{TelegramLimiter: limiter}
	previous := make([]*tele.Message, 8)
	parts := make([]string, 8)
	for index := range parts {
		previous[index] = &tele.Message{ID: index + 1, Chat: current.Chat()}
		parts[index] = fmt.Sprintf("Часть %d", index+1)
	}
	handler.rememberTrackedScheduleMessages(current, previous)
	if err = handler.replaceTrackedScheduleMessages(current, parts, keyboards.BackButton("refresh_schedule")); err != nil {
		t.Fatalf("long schedule replacement failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if sent != 8 || len(deleted) != 8 {
		t.Fatalf("sent=%d deleted=%v", sent, deleted)
	}
	if tracked := handler.scheduleMessages["42:108"]; len(tracked.IDs) != 8 {
		t.Fatalf("new schedule is not tracked: %+v", tracked)
	}
	if _, exists := handler.scheduleMessages["42:8"]; exists {
		t.Fatal("deleted schedule is still tracked")
	}
}

func TestPartialScheduleDeliveryPreservesOldScheduleAndTracksFailedCleanup(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanupFails), func(t *testing.T) {
			var mu sync.Mutex
			sent, restoredMarkup := 0, 0
			deleted := make([]string, 0)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
					if cleanupFails && body["message_id"] == "101" {
						fmt.Fprint(w, `{"ok":false,"error_code":500,"description":"injected cleanup failure"}`)
						return
					}
					deleted = append(deleted, body["message_id"])
					fmt.Fprint(w, `{"ok":true,"result":true}`)
					return
				case strings.HasSuffix(r.URL.Path, "/editMessageReplyMarkup"):
					restoredMarkup++
					fmt.Fprint(w, `{"ok":true,"result":{"message_id":101,"chat":{"id":42,"type":"private"}}}`)
					return
				default:
					sent++
					if sent == 3 {
						fmt.Fprint(w, `{"ok":false,"error_code":500,"description":"injected send failure"}`)
						return
					}
					fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"chat":{"id":42,"type":"private"}}}`, 100+sent)
				}
			}))
			defer server.Close()
			bot, err := tele.NewBot(tele.Settings{URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true})
			if err != nil {
				t.Fatal(err)
			}
			current := bot.NewContext(tele.Update{Callback: &tele.Callback{
				ID: "partial-schedule", Sender: &tele.User{ID: 42},
				Message: &tele.Message{ID: 8, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}},
			}})
			handler := &Handler{TelegramLimiter: telegramlimit.New(0, time.Millisecond)}
			handler.rememberTrackedScheduleMessages(current, []*tele.Message{current.Message()})
			if err = handler.replaceTrackedScheduleMessages(current, []string{"1", "2", "3"}, keyboards.BackButton("refresh_schedule")); err == nil {
				t.Fatal("send failure was ignored")
			}
			mu.Lock()
			defer mu.Unlock()
			if slices.Contains(deleted, "8") || !handler.hasTrackedScheduleMessages(current) {
				t.Fatal("old schedule lost on partial replacement")
			}
			if cleanupFails {
				if tracked := handler.scheduleMessages["42:101"]; len(tracked.IDs) != 1 || restoredMarkup != 1 {
					t.Fatalf("partial response orphaned: tracked=%+v markup=%d", tracked, restoredMarkup)
				}
			} else if !slices.Contains(deleted, "101") || !slices.Contains(deleted, "102") {
				t.Fatalf("partial response not cleaned: %v", deleted)
			}
		})
	}
}
