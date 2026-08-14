package v1

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	maxGroupsPerSnapshot  = 5_000
	maxLessonsPerSnapshot = 50_000
)

var (
	externalIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	timePattern       = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
)

type ValidationError struct {
	Problems []string `json:"problems"`
}

func (e *ValidationError) Error() string {
	return "connector snapshot is invalid: " + strings.Join(e.Problems, "; ")
}

func Validate(snapshot Snapshot) error {
	problems := make([]string, 0)
	add := func(format string, args ...any) {
		if len(problems) < 100 {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}
	if snapshot.SchemaVersion != SchemaVersion {
		add("schema_version must be %q", SchemaVersion)
	}
	if !validExternalID(snapshot.SnapshotID) {
		add("snapshot_id has an invalid format")
	}
	if snapshot.GeneratedAt.IsZero() {
		add("generated_at is required")
	} else if snapshot.GeneratedAt.After(time.Now().Add(10 * time.Minute)) {
		add("generated_at is too far in the future")
	}
	validateInstitution(snapshot.Institution, add)
	termStart, termEnd := validateTerm(snapshot.Term, add)
	if len(snapshot.Groups) == 0 {
		add("groups must contain at least one group")
	}
	if len(snapshot.Groups) > maxGroupsPerSnapshot {
		add("groups exceeds the limit of %d", maxGroupsPerSnapshot)
	}
	groupIDs := make(map[string]struct{}, len(snapshot.Groups))
	groupNames := make(map[string]struct{}, len(snapshot.Groups))
	lessonIDs := make(map[string]struct{})
	lessonCount := 0
	for groupIndex, group := range snapshot.Groups {
		path := fmt.Sprintf("groups[%d]", groupIndex)
		if !validExternalID(group.ExternalID) {
			add("%s.external_id has an invalid format", path)
		}
		if _, exists := groupIDs[group.ExternalID]; exists {
			add("%s.external_id is duplicated", path)
		}
		groupIDs[group.ExternalID] = struct{}{}
		name := strings.ToLower(strings.TrimSpace(group.Name))
		if name == "" {
			add("%s.name is required", path)
		} else if _, exists := groupNames[name]; exists {
			add("%s.name is duplicated", path)
		}
		groupNames[name] = struct{}{}
		for lessonIndex, lesson := range group.Lessons {
			lessonCount++
			validateLesson(
				lesson,
				fmt.Sprintf("%s.lessons[%d]", path, lessonIndex),
				termStart,
				termEnd,
				lessonIDs,
				add,
			)
		}
	}
	if lessonCount > maxLessonsPerSnapshot {
		add("lessons exceeds the limit of %d", maxLessonsPerSnapshot)
	}
	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func validateInstitution(value Institution, add func(string, ...any)) {
	if !validExternalID(value.ExternalID) {
		add("institution.external_id has an invalid format")
	}
	if strings.TrimSpace(value.Name) == "" {
		add("institution.name is required")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(value.Timezone)); err != nil {
		add("institution.timezone must be an IANA timezone")
	}
	if value.ScheduleURL != "" {
		parsed, err := url.ParseRequestURI(value.ScheduleURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			add("institution.schedule_url must be an absolute HTTP(S) URL")
		}
	}
}

func validateTerm(value Term, add func(string, ...any)) (time.Time, time.Time) {
	if !validExternalID(value.ExternalID) {
		add("term.external_id has an invalid format")
	}
	if strings.TrimSpace(value.Name) == "" {
		add("term.name is required")
	}
	start, startErr := time.Parse(time.DateOnly, value.StartsOn)
	end, endErr := time.Parse(time.DateOnly, value.EndsOn)
	if startErr != nil {
		add("term.starts_on must use YYYY-MM-DD")
	}
	if endErr != nil {
		add("term.ends_on must use YYYY-MM-DD")
	}
	if startErr == nil && endErr == nil && end.Before(start) {
		add("term.ends_on must not be before term.starts_on")
	}
	return start, end
}

func validateLesson(
	lesson Lesson,
	path string,
	termStart, termEnd time.Time,
	seen map[string]struct{},
	add func(string, ...any),
) {
	if !validExternalID(lesson.ExternalID) {
		add("%s.external_id has an invalid format", path)
	}
	if _, exists := seen[lesson.ExternalID]; exists {
		add("%s.external_id is duplicated in the snapshot", path)
	}
	seen[lesson.ExternalID] = struct{}{}
	if strings.TrimSpace(lesson.Subject) == "" {
		add("%s.subject is required", path)
	}
	allowedTypes := []string{"lecture", "practice", "lab", "seminar", "exam", "credit", "consultation"}
	if !slices.Contains(allowedTypes, lesson.Type) {
		add("%s.type is not supported", path)
	}
	if lesson.Subgroup < 0 || lesson.Subgroup > 100 {
		add("%s.subgroup must be between 0 and 100", path)
	}
	if !timePattern.MatchString(lesson.Schedule.StartsAt) || !timePattern.MatchString(lesson.Schedule.EndsAt) {
		add("%s.schedule time must use HH:MM", path)
	} else if lesson.Schedule.StartsAt >= lesson.Schedule.EndsAt {
		add("%s.schedule.ends_at must be after starts_at", path)
	}
	validateRecurrence(lesson.Schedule, path+".schedule", termStart, termEnd, add)
}

func validateRecurrence(schedule Schedule, path string, termStart, termEnd time.Time, add func(string, ...any)) {
	recurrence := schedule.Recurrence
	switch recurrence.Kind {
	case RecurrenceDate:
		date, err := time.Parse(time.DateOnly, schedule.Date)
		if err != nil {
			add("%s.date must use YYYY-MM-DD for date recurrence", path)
		} else if (!termStart.IsZero() && date.Before(termStart)) || (!termEnd.IsZero() && date.After(termEnd)) {
			add("%s.date is outside the term", path)
		}
		if schedule.DayOfWeek != 0 {
			add("%s.day_of_week must be omitted for date recurrence", path)
		}
	case RecurrenceEvery, RecurrenceOdd, RecurrenceEven:
		if schedule.DayOfWeek < 1 || schedule.DayOfWeek > 7 {
			add("%s.day_of_week must be between 1 and 7", path)
		}
		validateValidity(recurrence, path+".recurrence", termStart, termEnd, add)
	case RecurrenceCycle:
		if schedule.DayOfWeek < 1 || schedule.DayOfWeek > 7 {
			add("%s.day_of_week must be between 1 and 7", path)
		}
		if recurrence.CycleLength < 2 || recurrence.CycleLength > 16 {
			add("%s.recurrence.cycle_length must be between 2 and 16", path)
		}
		if len(recurrence.CycleWeeks) == 0 {
			add("%s.recurrence.cycle_weeks must not be empty", path)
		}
		seen := make(map[int]bool)
		for _, week := range recurrence.CycleWeeks {
			if week < 1 || week > recurrence.CycleLength {
				add("%s.recurrence.cycle_weeks contains an invalid week", path)
			}
			if seen[week] {
				add("%s.recurrence.cycle_weeks contains duplicates", path)
			}
			seen[week] = true
		}
		validateValidity(recurrence, path+".recurrence", termStart, termEnd, add)
	default:
		add("%s.recurrence.kind is not supported", path)
	}
}

func validateValidity(recurrence Recurrence, path string, termStart, termEnd time.Time, add func(string, ...any)) {
	if (recurrence.ValidFrom == "") != (recurrence.ValidTo == "") {
		add("%s.valid_from and valid_to must be supplied together", path)
		return
	}
	if recurrence.ValidFrom == "" {
		return
	}
	from, fromErr := time.Parse(time.DateOnly, recurrence.ValidFrom)
	to, toErr := time.Parse(time.DateOnly, recurrence.ValidTo)
	if fromErr != nil || toErr != nil {
		add("%s validity dates must use YYYY-MM-DD", path)
		return
	}
	if to.Before(from) {
		add("%s.valid_to must not be before valid_from", path)
	}
	if (!termStart.IsZero() && from.Before(termStart)) || (!termEnd.IsZero() && to.After(termEnd)) {
		add("%s validity must be inside the term", path)
	}
}

func validExternalID(value string) bool {
	return externalIDPattern.MatchString(strings.TrimSpace(value))
}

func IsValidationError(err error) bool {
	var target *ValidationError
	return errors.As(err, &target)
}
