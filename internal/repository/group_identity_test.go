package repository

import (
	"errors"
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
	if result.Groups[0].ExternalID != "isuct:group:23093" {
		t.Fatalf("external group id = %q", result.Groups[0].ExternalID)
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

	_, _, err := CanonicalizeSnapshotGroupIDs(payload, existing)
	if err == nil {
		t.Fatal("reused source id with a different name must be rejected")
	}
	var conflict *GroupIdentityConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error type = %T, want GroupIdentityConflictError", err)
	}
	if conflict.ExternalGroupID != "isuct:group:1" ||
		conflict.ExistingName != "1/1" || conflict.IncomingName != "2/2" {
		t.Fatalf("unexpected conflict: %+v", conflict)
	}
}

func TestCanonicalizeSnapshotGroupIDsUsesApprovedMapping(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		UniversityID: "ispu",
		Groups: []domain.SnapshotGroup{{
			ID: "ispu:group:101016", Name: "1-ЭЭ-В",
			Lessons: []domain.Lesson{{ID: "lesson", GroupID: "ispu:group:101016"}},
		}},
	}
	existing := []domain.Group{
		{ID: "ispu:group:101016", UniversityID: "ispu", Name: "2-ЭЭ-В", IsActive: true},
		{ID: "ispu:group:resolved:new", UniversityID: "ispu", Name: "1-ЭЭ-В"},
	}
	mappings := map[string]domain.GroupSourceIdentityMapping{
		"ispu:group:101016": {
			ExternalGroupID: "ispu:group:101016",
			GroupID:         "ispu:group:resolved:new",
			ExpectedName:    "1-ЭЭ-В",
		},
	}

	result, remapped, err := CanonicalizeSnapshotGroupIDsWithMappings(payload, existing, mappings)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 1 || result.Groups[0].ID != "ispu:group:resolved:new" {
		t.Fatalf("canonical group = %+v, remapped = %d", result.Groups[0], remapped)
	}
	if result.Groups[0].Lessons[0].GroupID != "ispu:group:resolved:new" {
		t.Fatalf("lesson group id = %q", result.Groups[0].Lessons[0].GroupID)
	}
}

func TestCanonicalizeSnapshotGroupIDsRejectsStaleApprovedMapping(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		UniversityID: "ispu",
		Groups:       []domain.SnapshotGroup{{ID: "ispu:group:101016", Name: "3-ЭЭ-В"}},
	}
	existing := []domain.Group{{
		ID: "ispu:group:resolved:new", UniversityID: "ispu", Name: "1-ЭЭ-В",
	}}
	mappings := map[string]domain.GroupSourceIdentityMapping{
		"ispu:group:101016": {
			ExternalGroupID: "ispu:group:101016",
			GroupID:         "ispu:group:resolved:new",
			ExpectedName:    "1-ЭЭ-В",
		},
	}

	_, _, err := CanonicalizeSnapshotGroupIDsWithMappings(payload, existing, mappings)
	var conflict *GroupIdentityConflictError
	if !errors.As(err, &conflict) || conflict.IncomingName != "3-ЭЭ-В" {
		t.Fatalf("unexpected stale mapping result: conflict=%+v err=%v", conflict, err)
	}
}

func TestCanonicalizeSnapshotDiscoversNameBasedMapping(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		Groups: []domain.SnapshotGroup{{
			ID:      "external-new-id",
			Name:    "1-ЭЭ-В",
			Lessons: []domain.Lesson{{GroupID: "external-new-id", Subject: "Математика"}},
		}},
	}
	existing := []domain.Group{{ID: "canonical-id", Name: "1-ЭЭ-В"}}
	canonical, remapped, mappings, err := canonicalizeSnapshotGroupIDs(payload, existing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 1 || canonical.Groups[0].ID != "canonical-id" || canonical.Groups[0].Lessons[0].GroupID != "canonical-id" {
		t.Fatalf("unexpected canonical snapshot: %+v", canonical)
	}
	if canonical.Groups[0].ExternalID != "external-new-id" {
		t.Fatalf("external group id = %q", canonical.Groups[0].ExternalID)
	}
	mapping, ok := mappings["external-new-id"]
	if !ok || mapping.GroupID != "canonical-id" || mapping.ExpectedName != "1-ЭЭ-В" {
		t.Fatalf("mapping was not discovered: %+v", mappings)
	}
}

func TestCanonicalizeSnapshotPreservesExternalIdentityAcrossStages(t *testing.T) {
	payload := domain.ScheduleSnapshot{
		Groups: []domain.SnapshotGroup{{
			ID:      "external-new-id",
			Name:    "1-ЭЭ-В",
			Lessons: []domain.Lesson{{GroupID: "external-new-id"}},
		}},
	}
	existing := []domain.Group{{ID: "canonical-id", Name: "1-ЭЭ-В"}}
	staged, _, _, err := canonicalizeSnapshotGroupIDs(payload, existing, nil)
	if err != nil {
		t.Fatal(err)
	}
	published, remapped, mappings, err := canonicalizeSnapshotGroupIDs(staged, existing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 0 || published.Groups[0].ID != "canonical-id" {
		t.Fatalf("published group = %+v, remapped = %d", published.Groups[0], remapped)
	}
	mapping, ok := mappings["external-new-id"]
	if !ok || mapping.GroupID != "canonical-id" {
		t.Fatalf("external mapping was lost between stages: %+v", mappings)
	}
}

func TestCanonicalizeSnapshotKeepsCanonicalIDForNewExternalGroup(t *testing.T) {
	payload := domain.ScheduleSnapshot{Groups: []domain.SnapshotGroup{{
		ID: "group:stable-hash", ExternalID: "source-group-101", Name: "1-ЭЭ-В",
		Lessons: []domain.Lesson{{GroupID: "group:stable-hash"}},
	}}}
	canonical, remapped, mappings, err := canonicalizeSnapshotGroupIDs(payload, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 0 || canonical.Groups[0].ID != "group:stable-hash" ||
		canonical.Groups[0].Lessons[0].GroupID != "group:stable-hash" {
		t.Fatalf("new external group lost canonical identity: %+v", canonical.Groups[0])
	}
	if mapping := mappings["source-group-101"]; mapping.GroupID != "group:stable-hash" {
		t.Fatalf("new external mapping = %+v", mapping)
	}
}
