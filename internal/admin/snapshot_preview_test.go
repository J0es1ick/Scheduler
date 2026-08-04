package admin

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestBuildSnapshotPreviewDetectsGroupAndLessonChanges(t *testing.T) {
	current := testSnapshot("current", []domain.SnapshotGroup{
		{ID: "old-g1", Name: "3/147", Lessons: []domain.Lesson{
			testSnapshotLesson("math", "09:00"),
			testSnapshotLesson("physics", "11:00"),
		}},
		{ID: "g2", Name: "3/148", Lessons: []domain.Lesson{
			testSnapshotLesson("english", "09:00"),
		}},
		{ID: "g4", Name: "3/150", Lessons: []domain.Lesson{
			testSnapshotLesson("history", "13:00"),
		}},
	})
	candidate := testSnapshot("candidate", []domain.SnapshotGroup{
		{ID: "new-g1", Name: "3/147", Lessons: []domain.Lesson{
			testSnapshotLesson("math", "09:00"),
			testSnapshotLesson("chemistry", "11:00"),
		}},
		{ID: "g3", Name: "3/149", Lessons: []domain.Lesson{
			testSnapshotLesson("programming", "15:00"),
		}},
		{ID: "g4", Name: "3/150", Lessons: []domain.Lesson{
			testSnapshotLesson("history", "13:00"),
		}},
	})

	preview := buildSnapshotPreview(candidate, current)
	if !preview.ComparisonAvailable {
		t.Fatal("comparison must be available")
	}
	if preview.Summary.AddedGroups != 1 ||
		preview.Summary.RemovedGroups != 1 ||
		preview.Summary.ChangedGroups != 1 ||
		preview.Summary.UnchangedGroups != 1 {
		t.Fatalf("unexpected summary: %+v", preview.Summary)
	}
	if preview.Summary.AddedLessons != 2 || preview.Summary.RemovedLessons != 2 {
		t.Fatalf("unexpected lesson delta: %+v", preview.Summary)
	}

	statuses := map[string]string{}
	for _, group := range preview.Groups {
		statuses[group.Name] = group.Status
	}
	for name, expected := range map[string]string{
		"3/147": snapshotGroupChanged,
		"3/148": snapshotGroupRemoved,
		"3/149": snapshotGroupAdded,
		"3/150": snapshotGroupUnchanged,
	} {
		if statuses[name] != expected {
			t.Errorf("group %s status=%q, want %q", name, statuses[name], expected)
		}
	}
}

func TestBuildSnapshotScheduleMarksBothSides(t *testing.T) {
	current := testSnapshot("current", []domain.SnapshotGroup{{
		ID: "g1", Name: "3/147", Lessons: []domain.Lesson{
			testSnapshotLesson("math", "09:00"),
			testSnapshotLesson("physics", "11:00"),
		},
	}})
	candidate := testSnapshot("candidate", []domain.SnapshotGroup{{
		ID: "g1", Name: "3/147", Lessons: []domain.Lesson{
			testSnapshotLesson("math", "09:00"),
			testSnapshotLesson("chemistry", "11:00"),
		},
	}})

	comparison := buildSnapshotSchedule(candidate, current, "g1")
	if comparison == nil {
		t.Fatal("comparison is nil")
	}
	if comparison.Status != snapshotGroupChanged {
		t.Fatalf("status=%q, want changed", comparison.Status)
	}
	if got := lessonDiffBySubject(comparison.Current); got["math"] != "unchanged" || got["physics"] != "removed" {
		t.Fatalf("unexpected current lesson states: %+v", got)
	}
	if got := lessonDiffBySubject(comparison.Candidate); got["math"] != "unchanged" || got["chemistry"] != "added" {
		t.Fatalf("unexpected candidate lesson states: %+v", got)
	}
}

func testSnapshot(id string, groups []domain.SnapshotGroup) *domain.ParserSnapshot {
	lessonCount := 0
	for _, group := range groups {
		lessonCount += len(group.Lessons)
	}
	return &domain.ParserSnapshot{
		ID:          id,
		GroupCount:  len(groups),
		LessonCount: lessonCount,
		Payload: domain.ScheduleSnapshot{
			Groups: groups,
		},
	}
}

func testSnapshotLesson(subject, start string) domain.Lesson {
	return domain.Lesson{
		DayOfWeek: 1,
		TimeStart: start,
		TimeEnd:   "10:30",
		WeekType:  domain.WeekTypeEvery,
		Subject:   subject,
		Type:      domain.LessonTypeLecture,
		Teacher:   "Преподаватель",
		Room:      "А-101",
	}
}

func lessonDiffBySubject(items []SnapshotLessonView) map[string]string {
	result := map[string]string{}
	for _, item := range items {
		result[item.Subject] = item.Diff
	}
	return result
}
