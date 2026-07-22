package repository

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestSupportAdminMessageFitsTelegramLimit(t *testing.T) {
	details := strings.Repeat("я", 4096)
	message := supportAdminMessage("request-id", domain.SupportRequestNewInstitution, "123456789", details)

	if size := utf8.RuneCountInString(message); size > 4096 {
		t.Fatalf("message has %d runes, Telegram limit is 4096", size)
	}
	if !strings.Contains(message, "полный текст сохранён в админке") {
		t.Fatal("truncated message does not explain where to find the full text")
	}
}
