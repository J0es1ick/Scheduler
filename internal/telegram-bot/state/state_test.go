package state

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func TestManagerReturnsStateCopy(t *testing.T) {
	manager := NewManager()
	original := &dto.UserState{UniversityID: "isuct", Step: "awaiting_query"}
	manager.Set(42, original)

	original.Step = "mutated outside manager"
	loaded := manager.Get(42)
	if loaded == nil || loaded.Step != "awaiting_query" {
		t.Fatalf("stored state was mutated through input pointer: %+v", loaded)
	}

	loaded.Step = "mutated returned copy"
	if reloaded := manager.Get(42); reloaded == nil || reloaded.Step != "awaiting_query" {
		t.Fatalf("stored state was mutated through returned pointer: %+v", reloaded)
	}
}

func TestManagerExpiresAbandonedDialogue(t *testing.T) {
	manager := NewManagerWithTTL(time.Minute)
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	manager.Set(42, &dto.UserState{Step: "awaiting_query"})

	now = now.Add(time.Minute)
	if state := manager.Get(42); state != nil {
		t.Fatalf("expired state = %+v, want nil", state)
	}
}
