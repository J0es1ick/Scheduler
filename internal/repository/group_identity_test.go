package repository

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestCanonicalizeSnapshotGroupIDsPreservesExistingIdentity(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		UniversityID: "isuct",
		Groups: []domain.SnapshotGroup{{
			ID: "isuct:group:23093", Name: "1/0",
			Lessons: []domain.Lesson{{ID: "lesson", GroupID: "isuct:group:23093"}},
		}},
	}
	existing := []domain.Group{{
		ID: "isuct:group:21299", UniversityID: "isuct", Name: "1/0", IsActive: true,
	}}

	result, remapped, err := CanonicalizeSnapshotGroupIDs(payload, existing)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 1 || result.Groups[0].ID != "isuct:group:21299" {
		t.Fatalf("canonical group = %+v, remapped = %d", result.Groups[0], remapped)
	}
	if result.Groups[0].Lessons[0].GroupID != "isuct:group:21299" {
		t.Fatalf("lesson group id = %q", result.Groups[0].Lessons[0].GroupID)
	}
}

func TestCanonicalizeSnapshotGroupIDsRejectsReusedID(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		UniversityID: "isuct",
		Groups:       []domain.SnapshotGroup{{ID: "isuct:group:1", Name: "2/2"}},
	}
	existing := []domain.Group{{
		ID: "isuct:group:1", UniversityID: "isuct", Name: "1/1", IsActive: true,
	}}

	if _, _, err := CanonicalizeSnapshotGroupIDs(payload, existing); err == nil {
		t.Fatal("reused source id with a different name must be rejected")
	}
}
