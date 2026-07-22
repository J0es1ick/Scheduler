package worker

import (
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
