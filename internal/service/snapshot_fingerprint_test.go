package service

import (
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestScheduleSnapshotsEquivalentIgnoresTechnicalIdentityAndOrder(t *testing.T) {
	validFrom := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.Local)
	validTo := time.Date(2026, time.December, 31, 0, 0, 0, 0, time.Local)
	first := domain.ScheduleSnapshot{
		UniversityID: "isuct",
		SemesterID:   "old-semester",
		StartDate:    validFrom,
		EndDate:      validTo,
		Groups: []domain.SnapshotGroup{
			{
				ID:   "old-group-2",
				Name: "3/148",
				Lessons: []domain.Lesson{{
					ID: "old-lesson-2", DayOfWeek: 2, TimeStart: "09:50", TimeEnd: "11:25",
					WeekType: domain.WeekTypeEven, Subject: "Химия", Type: domain.LessonTypeLecture,
					Teacher: "Иванов И.И.", Room: "А-101", ValidFrom: &validFrom, ValidTo: &validTo,
				}},
			},
			{
				ID:   "old-group-1",
				Name: "3/147",
				Lessons: []domain.Lesson{{
					ID: "old-lesson-1", DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35",
					WeekType: domain.WeekTypeOdd, Subject: "Математика", Type: domain.LessonTypePractice,
					Teacher: "Петров П.П.", Room: "Б-202", ValidFrom: &validFrom, ValidTo: &validTo,
				}},
			},
		},
	}
	second := domain.ScheduleSnapshot{
		UniversityID: "isuct",
		SemesterID:   "new-semester",
		StartDate:    validFrom.AddDate(0, 0, 1),
		EndDate:      validTo.AddDate(0, 0, 1),
		Groups: []domain.SnapshotGroup{
			{
				ID:   "new-group-1",
				Name: "3/147",
				Lessons: []domain.Lesson{{
					ID: "new-lesson-1", DayOfWeek: 1, TimeStart: "08:00", TimeEnd: "09:35",
					WeekType: domain.WeekTypeOdd, Subject: "  Математика ", Type: domain.LessonTypePractice,
					Teacher: "Петров  П.П.", Room: "Б-202", ValidFrom: &validFrom, ValidTo: &validTo,
				}},
			},
			{
				ID:   "new-group-2",
				Name: "3/148",
				Lessons: []domain.Lesson{{
					ID: "new-lesson-2", DayOfWeek: 2, TimeStart: "09:50", TimeEnd: "11:25",
					WeekType: domain.WeekTypeEven, Subject: "Химия", Type: domain.LessonTypeLecture,
					Teacher: "Иванов И.И.", Room: "А-101", ValidFrom: &validFrom, ValidTo: &validTo,
				}},
			},
		},
	}

	if !scheduleSnapshotsEquivalent(first, second) {
		t.Fatal("semantically equal snapshots must have the same fingerprint")
	}
	second.Groups[0].Lessons[0].Room = "Б-203"
	if scheduleSnapshotsEquivalent(first, second) {
		t.Fatal("a meaningful schedule change must change the fingerprint")
	}
}

func TestEvaluateSnapshotTrustsRepeatedApprovedEmptySchedule(t *testing.T) {
	approved := domain.ScheduleSnapshot{
		UniversityID: "isuct",
		Groups: []domain.SnapshotGroup{
			{ID: "g1", UniversityID: "isuct", Name: "1/1", Lessons: []domain.Lesson{}},
			{ID: "g2", UniversityID: "isuct", Name: "1/2", Lessons: []domain.Lesson{}},
		},
	}
	baseline := &domain.SnapshotBaseline{
		GroupCount:       2,
		LessonsByGroup:   map[string]int{"g1": 0, "g2": 0},
		CurrentSnapshot:  "approved-snapshot",
		TrustedSnapshot:  &approved,
		HasExistingState: true,
	}

	anomalies, publishable := evaluateSnapshot(approved, 0, baseline)
	if !publishable {
		t.Fatal("approved equivalent snapshot must remain publishable")
	}
	if len(anomalies) != 0 {
		t.Fatalf("approved equivalent snapshot must not return to quarantine: %+v", anomalies)
	}

	changed := approved
	changed.Groups = append([]domain.SnapshotGroup(nil), approved.Groups...)
	changed.Groups = changed.Groups[:1]
	anomalies, _ = evaluateSnapshot(changed, 0, baseline)
	if len(anomalies) == 0 {
		t.Fatal("changed empty snapshot must still be checked by quarantine rules")
	}
}
