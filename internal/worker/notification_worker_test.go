package worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestNotificationTextContainsScheduleContext(t *testing.T) {
	text := notificationText(domain.NotificationDelivery{
		UniversityName: "ИГХТУ",
		GroupName:      "3/42",
		Summary:        "Расписание обновлено: добавлено 2 занятия.",
	})
	for _, expected := range []string{"ИГХТУ", "3/42", "добавлено 2", "/week"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("notification text %q does not contain %q", text, expected)
		}
	}
}

func TestNotificationDigestSplitsWithoutDroppingDeliveries(t *testing.T) {
	items := make([]domain.NotificationDelivery, 25)
	for index := range items {
		items[index] = domain.NotificationDelivery{
			ID:             fmt.Sprintf("delivery-%02d", index),
			UniversityName: "Университет",
			GroupName:      fmt.Sprintf("Группа-%02d", index),
			Summary:        strings.Repeat(fmt.Sprintf("событие-%02d ", index), 70),
		}
	}

	batches := notificationDigestBatches(items)
	if len(batches) < 2 {
		t.Fatalf("got %d batch, want multiple Telegram messages", len(batches))
	}
	seen := make(map[string]int, len(items))
	for _, batch := range batches {
		if length := len([]rune(batch.Text)); length > notificationTelegramLimit {
			t.Fatalf("batch length = %d, limit = %d", length, notificationTelegramLimit)
		}
		for _, item := range batch.Items {
			seen[item.ID]++
		}
	}
	for _, item := range items {
		if seen[item.ID] != 1 {
			t.Fatalf("delivery %s included %d times", item.ID, seen[item.ID])
		}
	}
}

func TestNotificationRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, time.Minute},
		{2, 5 * time.Minute},
		{3, 30 * time.Minute},
		{4, 2 * time.Hour},
	}
	for _, test := range tests {
		if got := notificationRetryDelay(test.attempt); got != test.want {
			t.Fatalf("attempt %d: got %v, want %v", test.attempt, got, test.want)
		}
	}
}
