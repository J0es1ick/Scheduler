package bot

import (
	"context"
	"encoding/json"
	"errors"
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
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		requests++
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	telegramBot.Use(LimitOutboundByRecipient(ctx, telegramlimit.New(0, time.Hour)))
	var sendErrors []error
	telegramBot.Handle(tele.OnText, func(current tele.Context) error {
		err := current.Send("response")
		sendErrors = append(sendErrors, err)
		return err
	})
	for range 2 {
		telegramBot.ProcessUpdate(tele.Update{Message: &tele.Message{
			Text: "request", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
		}})
		cancel()
	}
	mutex.Lock()
	defer mutex.Unlock()
	if requests != 1 {
		t.Fatalf("Telegram requests: %d, want one before the cancelled wait", requests)
	}
	if len(sendErrors) != 2 || sendErrors[0] != nil || !errors.Is(sendErrors[1], context.Canceled) {
		t.Fatalf("send errors: %v, want success followed by cancelled recipient wait", sendErrors)
	}
}
