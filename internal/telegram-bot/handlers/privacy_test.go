package handlers

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func TestDeleteIntentIsSingleUseAndExpires(t *testing.T) {
	now := time.Date(2026, time.August, 26, 20, 0, 0, 0, time.UTC)
	state := &dto.UserState{
		PendingDeleteToken:     "one-time-token",
		PendingDeleteExpiresAt: now.Add(time.Minute),
	}
	if !consumeDeleteIntent(state, "one-time-token", now) {
		t.Fatal("valid deletion intent was rejected")
	}
	if consumeDeleteIntent(state, "one-time-token", now) {
		t.Fatal("deletion intent was accepted twice")
	}
	expired := &dto.UserState{
		PendingDeleteToken:     "expired-token",
		PendingDeleteExpiresAt: now,
	}
	if consumeDeleteIntent(expired, "expired-token", now) {
		t.Fatal("expired deletion intent was accepted")
	}
}
