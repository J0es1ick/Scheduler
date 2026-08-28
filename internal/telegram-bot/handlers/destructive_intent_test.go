package handlers

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
)

func TestConsumeSubscriptionDeleteIntentIsBoundAndOneShot(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	state := &dto.UserState{
		PendingSubscriptionDeleteToken:     "token",
		PendingSubscriptionDeleteGroupID:   "university:group:1",
		PendingSubscriptionDeleteExpiresAt: now.Add(time.Minute),
	}
	if consumeSubscriptionDeleteIntent(state, "token", keyboards.GroupToken("other"), now) {
		t.Fatal("intent accepted a different group")
	}
	if !consumeSubscriptionDeleteIntent(state, "token", keyboards.GroupToken("university:group:1"), now) {
		t.Fatal("valid intent was rejected")
	}
	if consumeSubscriptionDeleteIntent(state, "token", keyboards.GroupToken("university:group:1"), now) {
		t.Fatal("intent was accepted twice")
	}
}

func TestConsumeChatUnlinkIntentIsBoundAndOneShot(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	state := &dto.UserState{
		PendingChatUnlinkToken:     "token",
		PendingChatUnlinkChatID:    "-10042",
		PendingChatUnlinkExpiresAt: now.Add(time.Minute),
	}
	if consumeChatUnlinkIntent(state, "token", "-10043", now) {
		t.Fatal("intent accepted a different chat")
	}
	if !consumeChatUnlinkIntent(state, "token", "-10042", now) {
		t.Fatal("valid intent was rejected")
	}
	if consumeChatUnlinkIntent(state, "token", "-10042", now) {
		t.Fatal("intent was accepted twice")
	}
}
