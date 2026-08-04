package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scraper"
)

type diagnosticAdapter struct {
	calls atomic.Int32
}

func (a *diagnosticAdapter) Name() string         { return "test" }
func (a *diagnosticAdapter) UniversityID() string { return "test" }
func (a *diagnosticAdapter) SetSemesterID(string) {}

func (a *diagnosticAdapter) FetchGroups(
	context.Context,
) ([]domain.Group, error) {
	return nil, nil
}

func (a *diagnosticAdapter) FetchSchedule(
	_ context.Context,
	_ string,
) ([]domain.Lesson, error) {
	a.calls.Add(1)
	return nil, scraper.NewDiagnosticError(
		errors.New("empty AJAX response"),
		scraper.ResponseDiagnostic{
			Category:       "test.empty_response",
			Summary:        "Источник вернул пустой ответ",
			HTTPStatus:     200,
			ResponseSize:   3,
			ResponseSHA256: "same-response",
			Retryable:      false,
			StopBatch:      true,
		},
	)
}

func TestFetchSchedulesStopsAfterRepeatedDiagnostic(t *testing.T) {
	adapter := &diagnosticAdapter{}
	groups := make([]domain.Group, 10)
	for index := range groups {
		groups[index] = domain.Group{
			ID:   fmt.Sprintf("group-%d", index),
			Name: fmt.Sprintf("Group %d", index),
		}
	}

	report := (&ParserService{}).fetchSchedules(
		context.Background(),
		adapter,
		groups,
	)

	if report.Circuit == nil {
		t.Fatal("circuit was not opened")
	}
	if report.Attempted != scheduleCircuitThreshold {
		t.Errorf("attempted = %d, want %d", report.Attempted, scheduleCircuitThreshold)
	}
	if report.Failed != scheduleCircuitThreshold {
		t.Errorf("failed = %d, want %d", report.Failed, scheduleCircuitThreshold)
	}
	if report.Skipped != len(groups)-scheduleCircuitThreshold {
		t.Errorf(
			"skipped = %d, want %d",
			report.Skipped,
			len(groups)-scheduleCircuitThreshold,
		)
	}
	if calls := int(adapter.calls.Load()); calls != scheduleCircuitThreshold {
		t.Errorf("FetchSchedule calls = %d, want %d", calls, scheduleCircuitThreshold)
	}
	if message := report.Error(len(groups)).Error(); len(message) > 500 {
		t.Errorf("circuit error is unexpectedly long: %d characters", len(message))
	}
}
