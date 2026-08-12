package managedparser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scraper"
	managed "github.com/J0es1ick/Scheduler/parser/v1"
)

func Factory(factory managed.Factory) func(string) (scraper.SourceAdapter, error) {
	return func(_ string) (scraper.SourceAdapter, error) {
		if factory == nil {
			return nil, fmt.Errorf("managed parser factory is nil")
		}
		parser := factory()
		if parser == nil {
			return nil, fmt.Errorf("managed parser factory returned nil")
		}
		manifest := managed.NormalizeManifest(parser.Manifest())
		if err := managed.ValidateManifest(manifest); err != nil {
			return nil, err
		}
		return &adapter{
			parser:   parser,
			manifest: manifest,
			groups:   make(map[string]managed.Group),
		}, nil
	}
}

type adapter struct {
	parser     managed.Parser
	manifest   managed.Manifest
	semesterID string
	mu         sync.RWMutex
	groups     map[string]managed.Group
}

func (a *adapter) Name() string         { return a.manifest.DisplayName }
func (a *adapter) UniversityID() string { return a.manifest.Institution.ExternalID }
func (a *adapter) SetSemesterID(id string) {
	a.semesterID = id
}

func (a *adapter) FetchGroups(ctx context.Context) ([]domain.Group, error) {
	items, err := a.parser.FetchGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Group, 0, len(items))
	seenIDs := make(map[string]bool, len(items))
	seenNames := make(map[string]bool, len(items))
	lookup := make(map[string]managed.Group, len(items))
	for _, item := range items {
		item.ExternalID = strings.TrimSpace(item.ExternalID)
		item.Name = strings.TrimSpace(item.Name)
		if item.ExternalID == "" || item.Name == "" {
			return nil, fmt.Errorf("managed parser %s returned a group without stable id or name", a.manifest.ParserID)
		}
		if seenIDs[item.ExternalID] {
			return nil, fmt.Errorf("managed parser %s returned duplicate group id %q", a.manifest.ParserID, item.ExternalID)
		}
		nameKey := strings.ToLower(item.Name)
		if seenNames[nameKey] {
			return nil, fmt.Errorf("managed parser %s returned duplicate group name %q", a.manifest.ParserID, item.Name)
		}
		seenIDs[item.ExternalID] = true
		seenNames[nameKey] = true
		internalID := stableID("group", a.UniversityID(), item.ExternalID)
		lookup[internalID] = item
		result = append(result, domain.Group{
			ID: internalID, UniversityID: a.UniversityID(), Name: item.Name, IsActive: true,
		})
	}
	a.mu.Lock()
	a.groups = lookup
	a.mu.Unlock()
	return result, nil
}

func (a *adapter) FetchSchedule(ctx context.Context, groupID string) ([]domain.Lesson, error) {
	a.mu.RLock()
	group, ok := a.groups[groupID]
	a.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("managed parser %s does not know group %q", a.manifest.ParserID, groupID)
	}
	items, err := a.parser.FetchSchedule(ctx, group.ExternalID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := make([]domain.Lesson, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if seen[item.ExternalID] {
			return nil, fmt.Errorf("managed parser %s returned duplicate lesson id %q for group %q", a.manifest.ParserID, item.ExternalID, group.ExternalID)
		}
		seen[item.ExternalID] = true
		lesson, convertErr := convertLesson(a, groupID, item, now)
		if convertErr != nil {
			return nil, fmt.Errorf("managed parser %s lesson %q: %w", a.manifest.ParserID, item.ExternalID, convertErr)
		}
		result = append(result, lesson)
	}
	return result, nil
}

func convertLesson(a *adapter, groupID string, input managed.Lesson, fetchedAt time.Time) (domain.Lesson, error) {
	if strings.TrimSpace(input.ExternalID) == "" || strings.TrimSpace(input.Subject) == "" {
		return domain.Lesson{}, fmt.Errorf("external_id and subject are required")
	}
	lesson := domain.Lesson{
		ID:           stableID("lesson", a.manifest.ParserID, groupID, input.ExternalID),
		ExternalID:   input.ExternalID,
		UniversityID: a.UniversityID(),
		SemesterID:   a.semesterID,
		TimeStart:    input.Schedule.StartsAt,
		TimeEnd:      input.Schedule.EndsAt,
		Subject:      strings.TrimSpace(input.Subject),
		Type:         domain.LessonType(input.Type),
		Teacher:      strings.Join(clean(input.Teachers), ", "),
		Room:         strings.Join(clean(input.Rooms), ", "),
		GroupID:      groupID,
		Subgroup:     input.Subgroup,
		FetchedAt:    &fetchedAt,
		UpdatedAt:    fetchedAt,
	}
	recurrence := input.Schedule.Recurrence
	switch recurrence.Kind {
	case connector.RecurrenceDate:
		date, err := time.Parse(time.DateOnly, input.Schedule.Date)
		if err != nil {
			return domain.Lesson{}, fmt.Errorf("date must use YYYY-MM-DD: %w", err)
		}
		lesson.WeekType = domain.WeekTypeDate
		lesson.SpecialDate = &date
		lesson.ValidFrom = &date
		lesson.ValidTo = &date
	case connector.RecurrenceEvery, connector.RecurrenceOdd, connector.RecurrenceEven, connector.RecurrenceCycle:
		lesson.DayOfWeek = input.Schedule.DayOfWeek
		lesson.WeekType = domain.WeekType(recurrence.Kind)
		if recurrence.Kind == connector.RecurrenceCycle {
			lesson.WeekType = domain.WeekTypeEvery
			var anchor *time.Time
			if recurrence.ValidFrom != "" {
				parsed, err := time.Parse(time.DateOnly, recurrence.ValidFrom)
				if err != nil {
					return domain.Lesson{}, err
				}
				anchor = &parsed
			}
			lesson.Recurrence = domain.RecurrenceRule{
				CycleLength: recurrence.CycleLength,
				CycleWeeks:  append([]int(nil), recurrence.CycleWeeks...),
				AnchorDate:  anchor,
			}
		}
		if recurrence.ValidFrom != "" || recurrence.ValidTo != "" {
			if recurrence.ValidFrom == "" || recurrence.ValidTo == "" {
				return domain.Lesson{}, fmt.Errorf("valid_from and valid_to must be supplied together")
			}
			from, err := time.Parse(time.DateOnly, recurrence.ValidFrom)
			if err != nil {
				return domain.Lesson{}, err
			}
			to, err := time.Parse(time.DateOnly, recurrence.ValidTo)
			if err != nil {
				return domain.Lesson{}, err
			}
			lesson.ValidFrom, lesson.ValidTo = &from, &to
		}
	default:
		return domain.Lesson{}, fmt.Errorf("unsupported recurrence %q", recurrence.Kind)
	}
	fingerprint := sha256.Sum256([]byte(fmt.Sprintf("%#v", input)))
	lesson.SourceFingerprint = hex.EncodeToString(fingerprint[:])
	return lesson, nil
}

func stableID(kind string, values ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return kind + ":" + hex.EncodeToString(digest[:16])
}

func clean(values []string) []string {
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
