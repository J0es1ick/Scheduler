package repository

import "testing"

func TestCanAdoptReconciledLessonIDPreservesIncomingOwner(t *testing.T) {
	used := map[string]bool{}
	incoming := map[string]int{"existing-id": 1, "new-id": 1}

	if canAdoptReconciledLessonID("existing-id", "new-id", used, incoming) {
		t.Fatal("candidate owned by another incoming lesson was adopted")
	}
	if !canAdoptReconciledLessonID("existing-id", "existing-id", used, incoming) {
		t.Fatal("lesson could not retain its own stable identifier")
	}
	if !canAdoptReconciledLessonID("historic-id", "new-id", used, incoming) {
		t.Fatal("unreserved historic identifier was not adopted")
	}

	used["historic-id"] = true
	if canAdoptReconciledLessonID("historic-id", "another-id", used, incoming) {
		t.Fatal("already used identifier was adopted")
	}
}
