package service

import (
	"github.com/J0es1ick/Scheduler/internal/domain"
	"testing"
)

func TestTeacherChangesIncludeRemovedAndReplacementTeachers(t *testing.T) {
	before := []domain.Lesson{{GroupID: "one", Teacher: "Иванов И.И.", Subject: "Math"}}
	after := []domain.Lesson{{GroupID: "one", Teacher: "Петров П.П.", Subject: "Math"}}
	changes := TeacherChanges(before, after)
	if len(changes) != 2 || changes["иванов и и"] == "" || changes["петров п п"] == "" {
		t.Fatalf("changes %#v", changes)
	}
	after[0].Teacher = "ИВАНОВ И. И."
	if len(TeacherChanges(before, after)) != 0 {
		t.Fatal("format-only import notified teacher")
	}
	if len(TeacherChanges(before, nil)) != 1 {
		t.Fatal("removed lessons lost")
	}
}
