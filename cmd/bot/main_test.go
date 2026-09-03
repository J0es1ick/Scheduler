package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tgbotapi "gopkg.in/telebot.v3"
)

func TestPermanentTelegramMenuError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"blocked user", tgbotapi.ErrBlockedByUser, true},
		{"bad user", tgbotapi.ErrBadUserID, true},
		{"not found", tgbotapi.ErrNotFound, true},
		{"unknown typed server error", tgbotapi.NewError(500, "Server Error"), false},
		{"untyped bad request", errors.New("telegram: Bad Request (400)"), true},
		{"network error", errors.New("connection reset"), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := permanentTelegramMenuError(test.err); got != test.want {
				t.Fatalf("permanentTelegramMenuError(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}

type telegramFloodStub struct{}

func (telegramFloodStub) Error() string {
	return "rate limited"
}

func (telegramFloodStub) As(target any) bool {
	flood, ok := target.(*tgbotapi.FloodError)
	if ok {
		*flood = tgbotapi.FloodError{}
	}
	return ok
}

func TestTelegramHandlerFloodActivatesSharedBackoff(t *testing.T) {
	limiter := telegramlimit.New(0, 0)
	telegramErrorHandler(limiter)(telegramFloodStub{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := limiter.Wait(ctx, "worker-recipient"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shared backoff wait=%v", err)
	}
}

func TestTelegramHTTPClientAlwaysUsesLimitedTransport(t *testing.T) {
	client := newTelegramHTTPClient(telegramlimit.New(0, 0))
	if _, ok := client.Transport.(*telegramlimit.Transport); !ok {
		t.Fatalf("Telegram transport = %T", client.Transport)
	}
	if client.Timeout != 25*time.Second {
		t.Fatalf("Telegram timeout = %v", client.Timeout)
	}
}
