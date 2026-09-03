package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

func TestConfigureMiniAppMenuSharesFloodBackoff(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
	}))
	defer server.Close()

	bot, err := tele.NewBot(tele.Settings{
		URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	limiter := telegramlimit.New(0, 0)
	handler := &Handler{TelegramLimiter: limiter}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err = handler.configureMiniAppMenu(ctx, bot, &tele.User{ID: 42}, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("configure menu error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("Telegram calls = %d", calls.Load())
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer waitCancel()
	if err = limiter.Wait(waitCtx, "another-recipient"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared flood backoff error = %v", err)
	}
}
