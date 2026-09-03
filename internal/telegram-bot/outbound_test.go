package bot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

func TestLimitOutboundByRecipientWrapsContextSends(t *testing.T) {
	var mutex sync.Mutex
	var requests []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		requests = append(requests, time.Now())
		mutex.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"message_id": 1,
				"date":       1,
				"chat":       map[string]interface{}{"id": 42, "type": "private"},
			},
		})
	}))
	defer server.Close()

	telegramBot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	telegramBot.Use(LimitOutboundByRecipient(context.Background(), telegramlimit.New(0, 30*time.Millisecond)))
	telegramBot.Handle(tele.OnText, func(current tele.Context) error {
		return current.Send("response")
	})
	for range 2 {
		telegramBot.ProcessUpdate(tele.Update{Message: &tele.Message{
			Text: "request", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		}})
	}
	if len(requests) != 2 {
		t.Fatalf("Telegram requests: %d", len(requests))
	}
	if elapsed := requests[1].Sub(requests[0]); elapsed < 20*time.Millisecond {
		t.Fatalf("recipient requests were not spaced: %v", elapsed)
	}
}
