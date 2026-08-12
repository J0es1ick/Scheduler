package connectorapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
)

func convertSnapshot(sourceID, universityID string, input connector.Snapshot) (domain.ScheduleSnapshot, error) {
	termStart, err := time.Parse(time.DateOnly, input.Term.StartsOn)
	if err != nil {
		return domain.ScheduleSnapshot{}, err
	}
	termEnd, err := time.Parse(time.DateOnly, input.Term.EndsOn)
	if err != nil {
		return domain.ScheduleSnapshot{}, err
	}
	semesterID := stableID("term", universityID, input.Term.ExternalID)
	result := domain.ScheduleSnapshot{
		UniversityID: universityID,
		SemesterID:   semesterID,
		StartDate:    termStart,
		EndDate:      termEnd,
		Groups:       make([]domain.SnapshotGroup, 0, len(input.Groups)),
	}
	for _, inputGroup := range input.Groups {
		groupID := stableID("group", universityID, inputGroup.ExternalID)
		group := domain.SnapshotGroup{
			ID: groupID, UniversityID: universityID, Name: strings.TrimSpace(inputGroup.Name),
			Lessons: make([]domain.Lesson, 0, len(inputGroup.Lessons)),
		}
		for _, inputLesson := range inputGroup.Lessons {
			lesson, convertErr := convertLesson(
				sourceID, universityID, semesterID, groupID,
				input.GeneratedAt, termStart, termEnd, inputLesson,
			)
			if convertErr != nil {
				return domain.ScheduleSnapshot{}, fmt.Errorf("lesson %s: %w", inputLesson.ExternalID, convertErr)
			}
			group.Lessons = append(group.Lessons, lesson)
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

func convertLesson(
	sourceID, universityID, semesterID, groupID string,
	fetchedAt, termStart, termEnd time.Time,
	input connector.Lesson,
) (domain.Lesson, error) {
	lesson := domain.Lesson{
		ID:           stableID("lesson", sourceID, groupID, input.ExternalID),
		ExternalID:   input.ExternalID,
		UniversityID: universityID,
		SemesterID:   semesterID,
		TimeStart:    input.Schedule.StartsAt,
		TimeEnd:      input.Schedule.EndsAt,
		Subject:      strings.TrimSpace(input.Subject),
		Type:         domain.LessonType(input.Type),
		Teacher:      strings.Join(cleanValues(input.Teachers), ", "),
		Room:         strings.Join(cleanValues(input.Rooms), ", "),
		GroupID:      groupID,
		Subgroup:     input.Subgroup,
		SourceID:     sourceID,
		FetchedAt:    &fetchedAt,
		UpdatedAt:    time.Now(),
	}
	recurrence := input.Schedule.Recurrence
	if recurrence.Kind == connector.RecurrenceDate {
		date, err := time.Parse(time.DateOnly, input.Schedule.Date)
		if err != nil {
			return domain.Lesson{}, err
		}
		lesson.WeekType = domain.WeekTypeDate
		lesson.SpecialDate = &date
		lesson.ValidFrom = &date
		lesson.ValidTo = &date
	} else {
		lesson.DayOfWeek = input.Schedule.DayOfWeek
		lesson.WeekType = domain.WeekType(recurrence.Kind)
		if recurrence.Kind == connector.RecurrenceCycle {
			lesson.WeekType = domain.WeekTypeEvery
			anchor := termStart
			lesson.Recurrence = domain.RecurrenceRule{
				CycleLength: recurrence.CycleLength,
				CycleWeeks:  append([]int(nil), recurrence.CycleWeeks...),
				AnchorDate:  &anchor,
			}
		}
		validFrom, validTo := termStart, termEnd
		if recurrence.ValidFrom != "" {
			var err error
			validFrom, err = time.Parse(time.DateOnly, recurrence.ValidFrom)
			if err != nil {
				return domain.Lesson{}, err
			}
			validTo, err = time.Parse(time.DateOnly, recurrence.ValidTo)
			if err != nil {
				return domain.Lesson{}, err
			}
		}
		lesson.ValidFrom = &validFrom
		lesson.ValidTo = &validTo
	}
	fingerprintPayload := struct {
		ExternalID string
		Subject    string
		Type       string
		Teachers   []string
		Rooms      []string
		Subgroup   int
		Schedule   connector.Schedule
	}{input.ExternalID, input.Subject, input.Type, input.Teachers, input.Rooms, input.Subgroup, input.Schedule}
	encoded, _ := json.Marshal(fingerprintPayload)
	digest := sha256.Sum256(encoded)
	lesson.SourceFingerprint = hex.EncodeToString(digest[:])
	return lesson, nil
}

func stableID(kind string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return kind + ":" + hex.EncodeToString(digest[:16])
}

func cleanValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
