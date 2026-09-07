package admin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestReleaseLogoutAvailableToEveryAdminRole(t *testing.T) {
	for _, role := range []string{"read_only", "support", "editor", "reviewer", "operator", "owner"} {
		t.Run(role, func(t *testing.T) {
			if !roleAllows(role, roleForPattern("POST /api/auth/logout")) {
				t.Fatalf("authenticated %s cannot log out", role)
			}
		})
	}
}
func TestReleaseEditorPreservesRecurrenceInReadModel(t *testing.T) {
	anchor := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	lesson := domain.Lesson{Recurrence: domain.RecurrenceRule{CycleLength: 3, CycleWeeks: []int{1}, AnchorDate: &anchor}}
	raw, err := json.Marshal(lesson)
	if err != nil {
		t.Fatal(err)
	}
	var model EditorLesson
	if err = json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var restored domain.Lesson
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Recurrence.IsZero() {
		t.Fatal("EditorLesson drops the source recurrence")
	}
}

type unavailableTelegramChecker struct{}

func (unavailableTelegramChecker) TelegramAdmin(context.Context, string) (*UserView, error) {
	return nil, errors.New("synthetic database unavailable")
}
func TestTelegramLoginDistinguishesDatabaseOutage(t *testing.T) {
	const token = "synthetic-token"
	auth := NewAuthManager(token, "", false, false)
	raw := signedTelegramInitData(t, token, time.Now(), `{"id":42}`)
	_, err := auth.LoginWithTelegram(context.Background(), unavailableTelegramChecker{}, raw)
	if err == nil || errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("database outage disguised as access denial: %v", err)
	}
}
