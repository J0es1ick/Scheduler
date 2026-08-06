package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestFormatServiceMetricsIncludesReminderWorkerState(t *testing.T) {
	finishedAt := time.Date(2026, time.August, 4, 16, 30, 0, 0, time.Local)
	fullCycleAt := finishedAt.Add(-time.Minute)
	text := formatServiceMetrics(&domain.ServiceMetrics{
		Users: 12,
		ReminderWorker: domain.WorkerStatus{
			Cursor:          "cursor",
			LastFinishedAt:  &finishedAt,
			LastFullCycleAt: &fullCycleAt,
			LastDurationMS:  1250,
			LastProcessed:   250,
			LastFailures:    1,
			LastError:       "temporary database error",
		},
	})

	for _, expected := range []string{
		"Пользователи: 12",
		"получателей: 250",
		"ошибок: 1",
		"1.2 сек.",
		"обход будет продолжен",
		"Последняя ошибка напоминаний: temporary database error",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("metrics text does not contain %q:\n%s", expected, text)
		}
	}
}
