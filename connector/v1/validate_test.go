package v1

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"
)

func validSnapshot() Snapshot {
	return Snapshot{
		SchemaVersion: SchemaVersion,
		SnapshotID:    "snapshot-1",
		GeneratedAt:   time.Now().UTC(),
		Institution:   Institution{ExternalID: "test", Name: "Test", Timezone: "Europe/Moscow"},
		Term:          Term{ExternalID: "2026-autumn", Name: "Autumn", StartsOn: "2026-09-01", EndsOn: "2027-01-31"},
		Groups: []Group{{
			ExternalID: "group-1", Name: "1/1",
			Lessons: []Lesson{{
				ExternalID: "lesson-1", Subject: "Math", Type: "lecture",
				Schedule: Schedule{DayOfWeek: 1, StartsAt: "09:00", EndsAt: "10:30", Recurrence: Recurrence{Kind: RecurrenceOdd}},
			}},
		}},
	}
}

func TestValidateAcceptsVersionOneSnapshot(t *testing.T) {
	if err := Validate(validSnapshot()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidCycle(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Groups[0].Lessons[0].Schedule.Recurrence = Recurrence{
		Kind: RecurrenceCycle, CycleLength: 4, CycleWeeks: []int{5},
	}
	if err := Validate(snapshot); err == nil {
		t.Fatal("Validate() accepted an invalid cycle")
	}
}

func TestValidateMaximumSupportedSnapshot(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Groups[0].Lessons = make([]Lesson, maxLessonsPerSnapshot)
	baseLesson := validSnapshot().Groups[0].Lessons[0]
	for index := range snapshot.Groups[0].Lessons {
		lesson := baseLesson
		lesson.ExternalID = fmt.Sprintf("lesson-%d", index)
		snapshot.Groups[0].Lessons[index] = lesson
	}
	if err := Validate(snapshot); err != nil {
		t.Fatalf("maximum snapshot was rejected: %v", err)
	}
	extra := snapshot.Groups[0].Lessons[0]
	extra.ExternalID = "lesson-over-limit"
	snapshot.Groups[0].Lessons = append(snapshot.Groups[0].Lessons, extra)
	if err := Validate(snapshot); err == nil {
		t.Fatal("snapshot above the lesson limit was accepted")
	}
}

func TestRequestSignature(t *testing.T) {
	publicEncoded, privateEncoded, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _ := DecodePublicKey(publicEncoded)
	privateKey, _ := DecodePrivateKey(privateEncoded)
	body := []byte(`{"ok":true}`)
	signature := SignRequest(privateKey, "POST", "/snapshot", "now", "nonce", body)
	if !VerifyRequest(publicKey, "POST", "/snapshot", "now", "nonce", PayloadDigest(body), signature) {
		t.Fatal("valid signature was rejected")
	}
	if VerifyRequest(ed25519.PublicKey(publicKey), "POST", "/snapshot", "now", "nonce", PayloadDigest([]byte("changed")), signature) {
		t.Fatal("signature accepted a changed payload")
	}
}
