package service

import (
	"github.com/J0es1ick/Scheduler/internal/domain"
	"testing"
	"time"
)

func TestPersonalPatchValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		patch domain.PersonalLessonPatch
		valid bool
	}{
		{"empty", domain.PersonalLessonPatch{}, true},
		{"blank subject", domain.PersonalLessonPatch{Subject: new("")}, false},
		{"empty room", domain.PersonalLessonPatch{Room: new("")}, true},
		{"newline", domain.PersonalLessonPatch{Teacher: new("name\nname")}, false},
		{"half time", domain.PersonalLessonPatch{TimeStart: new("09:00")}, false},
		{"bad time", domain.PersonalLessonPatch{TimeStart: new("29:00"), TimeEnd: new("30:00")}, false},
		{"reverse time", domain.PersonalLessonPatch{TimeStart: new("10:00"), TimeEnd: new("09:00")}, false},
		{"time", domain.PersonalLessonPatch{TimeStart: new("09:00"), TimeEnd: new("10:00")}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePersonalPatch(test.patch); (err == nil) != test.valid {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}
func TestPersonalOverrideIdentityAndLocalDate(t *testing.T) {
	date := time.Date(2026, 10, 5, 0, 0, 0, 0, time.FixedZone("UTC+12", 12*3600))
	original := domain.Lesson{ID: "override-id", PersonalKey: "stable-id", SemesterID: "term", Room: "base"}
	changes := []domain.PersonalOverride{{LessonID: "stable-id", SemesterID: "term", ValidFrom: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), ValidTo: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Patch: []byte(`{"room":"personal"}`)}}
	got, err := applyPersonalLesson(original, date, changes)
	if err != nil || got.Lesson.Room != "personal" || original.Room != "base" {
		t.Fatalf("local date or stable identity: %+v %v", got, err)
	}
	original.SemesterID = "other"
	got, err = applyPersonalLesson(original, date, changes)
	if err != nil || got.Lesson.Room != "base" {
		t.Fatalf("term leaked: %+v %v", got, err)
	}
}
