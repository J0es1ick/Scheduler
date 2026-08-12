package main

import (
	"errors"
	"testing"

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
