package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

type snapshotFingerprintDocument struct {
	Groups []snapshotFingerprintGroup `json:"groups"`
}

type snapshotFingerprintGroup struct {
	Name    string            `json:"name"`
	Lessons []json.RawMessage `json:"lessons"`
}

type snapshotFingerprintLesson struct {
	DayOfWeek   int                   `json:"day_of_week"`
	SpecialDate string                `json:"special_date"`
	TimeStart   string                `json:"time_start"`
	TimeEnd     string                `json:"time_end"`
	WeekType    domain.WeekType       `json:"week_type"`
	Subject     string                `json:"subject"`
	Type        domain.LessonType     `json:"type"`
	Teacher     string                `json:"teacher"`
	Room        string                `json:"room"`
	Subgroup    int                   `json:"subgroup"`
	ValidFrom   string                `json:"valid_from"`
	ValidTo     string                `json:"valid_to"`
	Recurrence  domain.RecurrenceRule `json:"recurrence,omitempty"`
	ExternalID  string                `json:"external_id,omitempty"`
}

func scheduleSnapshotsEquivalent(left, right domain.ScheduleSnapshot) bool {
	return scheduleSnapshotFingerprint(left) == scheduleSnapshotFingerprint(right)
}

func scheduleSnapshotFingerprint(snapshot domain.ScheduleSnapshot) string {
	document := snapshotFingerprintDocument{
		Groups: make([]snapshotFingerprintGroup, 0, len(snapshot.Groups)),
	}
	for _, group := range snapshot.Groups {
		fingerprintGroup := snapshotFingerprintGroup{
			Name:    normalizeFingerprintText(group.Name),
			Lessons: make([]json.RawMessage, 0, len(group.Lessons)),
		}
		for _, lesson := range group.Lessons {
			encoded, _ := json.Marshal(snapshotFingerprintLesson{
				DayOfWeek:   lesson.DayOfWeek,
				SpecialDate: snapshotFingerprintDate(lesson.SpecialDate),
				TimeStart:   strings.TrimSpace(lesson.TimeStart),
				TimeEnd:     strings.TrimSpace(lesson.TimeEnd),
				WeekType:    lesson.WeekType,
				Subject:     normalizeFingerprintText(lesson.Subject),
				Type:        lesson.Type,
				Teacher:     normalizeFingerprintText(lesson.Teacher),
				Room:        normalizeFingerprintText(lesson.Room),
				Subgroup:    lesson.Subgroup,
				ValidFrom:   snapshotFingerprintDate(lesson.ValidFrom),
				ValidTo:     snapshotFingerprintDate(lesson.ValidTo),
				Recurrence:  lesson.Recurrence,
				ExternalID:  strings.TrimSpace(lesson.ExternalID),
			})
			fingerprintGroup.Lessons = append(fingerprintGroup.Lessons, encoded)
		}
		sort.Slice(fingerprintGroup.Lessons, func(i, j int) bool {
			return string(fingerprintGroup.Lessons[i]) < string(fingerprintGroup.Lessons[j])
		})
		document.Groups = append(document.Groups, fingerprintGroup)
	}
	sort.Slice(document.Groups, func(i, j int) bool {
		return document.Groups[i].Name < document.Groups[j].Name
	})
	encoded, _ := json.Marshal(document)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func normalizeFingerprintText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func snapshotFingerprintDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}
