package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

const (
	snapshotGroupAdded     = "added"
	snapshotGroupRemoved   = "removed"
	snapshotGroupChanged   = "changed"
	snapshotGroupUnchanged = "unchanged"
)

type SnapshotComparisonSummary struct {
	AddedGroups     int `json:"added_groups"`
	RemovedGroups   int `json:"removed_groups"`
	ChangedGroups   int `json:"changed_groups"`
	UnchangedGroups int `json:"unchanged_groups"`
	AddedLessons    int `json:"added_lessons"`
	RemovedLessons  int `json:"removed_lessons"`
}

type SnapshotGroupDiff struct {
	ID               string `json:"id"`
	CurrentID        string `json:"current_id"`
	CandidateID      string `json:"candidate_id"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	CurrentLessons   int    `json:"current_lessons"`
	CandidateLessons int    `json:"candidate_lessons"`
	AddedLessons     int    `json:"added_lessons"`
	RemovedLessons   int    `json:"removed_lessons"`
}

type SnapshotPreview struct {
	SnapshotID           string                    `json:"snapshot_id"`
	DataSourceID         string                    `json:"data_source_id"`
	Status               string                    `json:"status"`
	Publishable          bool                      `json:"publishable"`
	CreatedAt            time.Time                 `json:"created_at"`
	CandidateStartDate   time.Time                 `json:"candidate_start_date"`
	CandidateEndDate     time.Time                 `json:"candidate_end_date"`
	CandidateGroupCount  int                       `json:"candidate_group_count"`
	CandidateLessonCount int                       `json:"candidate_lesson_count"`
	CurrentSnapshotID    string                    `json:"current_snapshot_id"`
	CurrentCreatedAt     *time.Time                `json:"current_created_at"`
	CurrentGroupCount    int                       `json:"current_group_count"`
	CurrentLessonCount   int                       `json:"current_lesson_count"`
	ComparisonAvailable  bool                      `json:"comparison_available"`
	Summary              SnapshotComparisonSummary `json:"summary"`
	Groups               []SnapshotGroupDiff       `json:"groups"`
}

type SnapshotLessonView struct {
	ID          string     `json:"id"`
	DayOfWeek   int        `json:"day_of_week"`
	SpecialDate *time.Time `json:"special_date"`
	TimeStart   string     `json:"time_start"`
	TimeEnd     string     `json:"time_end"`
	WeekType    string     `json:"week_type"`
	Subject     string     `json:"subject"`
	Type        string     `json:"type"`
	Teacher     string     `json:"teacher"`
	Room        string     `json:"room"`
	Subgroup    int        `json:"subgroup"`
	ValidFrom   *time.Time `json:"valid_from"`
	ValidTo     *time.Time `json:"valid_to"`
	Diff        string     `json:"diff"`
}

type SnapshotScheduleComparison struct {
	SnapshotID          string               `json:"snapshot_id"`
	GroupID             string               `json:"group_id"`
	GroupName           string               `json:"group_name"`
	Status              string               `json:"status"`
	ComparisonAvailable bool                 `json:"comparison_available"`
	Current             []SnapshotLessonView `json:"current"`
	Candidate           []SnapshotLessonView `json:"candidate"`
}

func (s *Store) ParserSnapshotPreview(ctx context.Context, snapshotID string) (*SnapshotPreview, error) {
	repo := repository.NewParserSnapshotRepository(s.db)
	candidate, err := repo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, ErrNotFound
	}

	current, currentID, err := s.currentParserSnapshot(ctx, repo, candidate.DataSourceID)
	if err != nil {
		return nil, err
	}
	preview := buildSnapshotPreview(candidate, current)
	preview.CurrentSnapshotID = currentID
	return preview, nil
}

func (s *Store) ParserSnapshotSchedule(
	ctx context.Context,
	snapshotID, groupID string,
) (*SnapshotScheduleComparison, error) {
	repo := repository.NewParserSnapshotRepository(s.db)
	candidate, err := repo.Get(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, ErrNotFound
	}
	current, _, err := s.currentParserSnapshot(ctx, repo, candidate.DataSourceID)
	if err != nil {
		return nil, err
	}
	comparison := buildSnapshotSchedule(candidate, current, groupID)
	if comparison == nil {
		return nil, ErrNotFound
	}
	return comparison, nil
}

func (s *Store) currentParserSnapshot(
	ctx context.Context,
	repo *repository.ParserSnapshotRepository,
	sourceID string,
) (*domain.ParserSnapshot, string, error) {
	var currentID string
	err := s.db.GetContext(ctx, &currentID, `
		SELECT COALESCE(current_snapshot_id, '')
		FROM data_sources WHERE id=$1`, sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("load current parser snapshot id: %w", err)
	}
	if currentID == "" {
		return nil, "", nil
	}
	current, err := repo.Get(ctx, currentID)
	if err != nil {
		return nil, currentID, err
	}
	return current, currentID, nil
}

func buildSnapshotPreview(candidate, current *domain.ParserSnapshot) *SnapshotPreview {
	preview := &SnapshotPreview{
		SnapshotID:           candidate.ID,
		DataSourceID:         candidate.DataSourceID,
		Status:               candidate.Status,
		Publishable:          candidate.Publishable,
		CreatedAt:            candidate.CreatedAt,
		CandidateStartDate:   candidate.Payload.StartDate,
		CandidateEndDate:     candidate.Payload.EndDate,
		CandidateGroupCount:  candidate.GroupCount,
		CandidateLessonCount: candidate.LessonCount,
		ComparisonAvailable:  current != nil,
		Groups:               []SnapshotGroupDiff{},
	}
	if current != nil {
		preview.CurrentSnapshotID = current.ID
		preview.CurrentCreatedAt = &current.CreatedAt
		preview.CurrentGroupCount = current.GroupCount
		preview.CurrentLessonCount = current.LessonCount
	}

	candidateGroups := snapshotGroupsByKey(candidate)
	currentGroups := snapshotGroupsByKey(current)
	groupIDs := make(map[string]struct{}, len(candidateGroups)+len(currentGroups))
	for id := range candidateGroups {
		groupIDs[id] = struct{}{}
	}
	for id := range currentGroups {
		groupIDs[id] = struct{}{}
	}

	for key := range groupIDs {
		candidateGroup, inCandidate := candidateGroups[key]
		currentGroup, inCurrent := currentGroups[key]
		diff := compareSnapshotGroup(candidateGroup, inCandidate, currentGroup, inCurrent)
		preview.Groups = append(preview.Groups, diff)
		preview.Summary.AddedLessons += diff.AddedLessons
		preview.Summary.RemovedLessons += diff.RemovedLessons
		switch diff.Status {
		case snapshotGroupAdded:
			preview.Summary.AddedGroups++
		case snapshotGroupRemoved:
			preview.Summary.RemovedGroups++
		case snapshotGroupChanged:
			preview.Summary.ChangedGroups++
		default:
			preview.Summary.UnchangedGroups++
		}
	}
	sort.Slice(preview.Groups, func(i, j int) bool {
		left, right := preview.Groups[i], preview.Groups[j]
		if snapshotGroupRank(left.Status) != snapshotGroupRank(right.Status) {
			return snapshotGroupRank(left.Status) < snapshotGroupRank(right.Status)
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	return preview
}

func buildSnapshotSchedule(
	candidate, current *domain.ParserSnapshot,
	groupID string,
) *SnapshotScheduleComparison {
	candidateGroup, inCandidate := snapshotGroupByID(candidate, groupID)
	currentGroup, inCurrent := snapshotGroupByID(current, groupID)
	if inCandidate {
		currentGroup, inCurrent = snapshotGroupsByKey(current)[snapshotGroupKey(candidateGroup)]
	} else if inCurrent {
		candidateGroup, inCandidate = snapshotGroupsByKey(candidate)[snapshotGroupKey(currentGroup)]
	}
	if !inCandidate && !inCurrent {
		return nil
	}
	diff := compareSnapshotGroup(candidateGroup, inCandidate, currentGroup, inCurrent)
	name := candidateGroup.Name
	if name == "" {
		name = currentGroup.Name
	}
	result := &SnapshotScheduleComparison{
		SnapshotID:          candidate.ID,
		GroupID:             groupID,
		GroupName:           name,
		Status:              diff.Status,
		ComparisonAvailable: current != nil,
		Current:             []SnapshotLessonView{},
		Candidate:           []SnapshotLessonView{},
	}
	if inCurrent {
		result.Current = snapshotLessonViews(currentGroup.Lessons, candidateGroup.Lessons, "removed")
	}
	if inCandidate {
		result.Candidate = snapshotLessonViews(candidateGroup.Lessons, currentGroup.Lessons, "added")
	}
	return result
}

func compareSnapshotGroup(
	candidate domain.SnapshotGroup,
	inCandidate bool,
	current domain.SnapshotGroup,
	inCurrent bool,
) SnapshotGroupDiff {
	name := candidate.Name
	if name == "" {
		name = current.Name
	}
	result := SnapshotGroupDiff{
		ID:               candidate.ID,
		CurrentID:        current.ID,
		CandidateID:      candidate.ID,
		Name:             name,
		CurrentLessons:   len(current.Lessons),
		CandidateLessons: len(candidate.Lessons),
	}
	if result.ID == "" {
		result.ID = current.ID
	}
	result.AddedLessons, result.RemovedLessons = snapshotLessonDelta(
		candidate.Lessons,
		current.Lessons,
	)
	switch {
	case inCandidate && !inCurrent:
		result.Status = snapshotGroupAdded
	case !inCandidate && inCurrent:
		result.Status = snapshotGroupRemoved
	case candidate.Name != current.Name || result.AddedLessons > 0 || result.RemovedLessons > 0:
		result.Status = snapshotGroupChanged
	default:
		result.Status = snapshotGroupUnchanged
	}
	return result
}

func snapshotGroupsByKey(snapshot *domain.ParserSnapshot) map[string]domain.SnapshotGroup {
	result := map[string]domain.SnapshotGroup{}
	if snapshot == nil {
		return result
	}
	for _, group := range snapshot.Payload.Groups {
		result[snapshotGroupKey(group)] = group
	}
	return result
}

func snapshotGroupByID(snapshot *domain.ParserSnapshot, groupID string) (domain.SnapshotGroup, bool) {
	if snapshot == nil {
		return domain.SnapshotGroup{}, false
	}
	for _, group := range snapshot.Payload.Groups {
		if group.ID == groupID {
			return group, true
		}
	}
	return domain.SnapshotGroup{}, false
}

func snapshotGroupKey(group domain.SnapshotGroup) string {
	name := strings.ToLower(strings.TrimSpace(group.Name))
	if name != "" {
		return name
	}
	return "id:" + group.ID
}

func snapshotLessonDelta(candidate, current []domain.Lesson) (added, removed int) {
	currentCounts := snapshotLessonCounts(current)
	for _, lesson := range candidate {
		signature := snapshotLessonSignature(lesson)
		if currentCounts[signature] > 0 {
			currentCounts[signature]--
		} else {
			added++
		}
	}
	for _, count := range currentCounts {
		removed += count
	}
	return added, removed
}

func snapshotLessonViews(
	lessons, reference []domain.Lesson,
	missingState string,
) []SnapshotLessonView {
	referenceCounts := snapshotLessonCounts(reference)
	result := make([]SnapshotLessonView, 0, len(lessons))
	for _, lesson := range lessons {
		signature := snapshotLessonSignature(lesson)
		state := missingState
		if referenceCounts[signature] > 0 {
			state = "unchanged"
			referenceCounts[signature]--
		}
		result = append(result, SnapshotLessonView{
			ID:          lesson.ID,
			DayOfWeek:   lesson.DayOfWeek,
			SpecialDate: lesson.SpecialDate,
			TimeStart:   lesson.TimeStart,
			TimeEnd:     lesson.TimeEnd,
			WeekType:    string(lesson.WeekType),
			Subject:     lesson.Subject,
			Type:        string(lesson.Type),
			Teacher:     lesson.Teacher,
			Room:        lesson.Room,
			Subgroup:    lesson.Subgroup,
			ValidFrom:   lesson.ValidFrom,
			ValidTo:     lesson.ValidTo,
			Diff:        state,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.DayOfWeek != right.DayOfWeek {
			return left.DayOfWeek < right.DayOfWeek
		}
		if datePart(left.SpecialDate) != datePart(right.SpecialDate) {
			return datePart(left.SpecialDate) < datePart(right.SpecialDate)
		}
		if left.TimeStart != right.TimeStart {
			return left.TimeStart < right.TimeStart
		}
		return left.Subject < right.Subject
	})
	return result
}

func snapshotLessonCounts(lessons []domain.Lesson) map[string]int {
	result := make(map[string]int, len(lessons))
	for _, lesson := range lessons {
		result[snapshotLessonSignature(lesson)]++
	}
	return result
}

func snapshotLessonSignature(lesson domain.Lesson) string {
	parts := []string{
		strconv.Itoa(lesson.DayOfWeek),
		datePart(lesson.SpecialDate),
		strings.TrimSpace(lesson.TimeStart),
		strings.TrimSpace(lesson.TimeEnd),
		string(lesson.WeekType),
		strings.TrimSpace(lesson.Subject),
		string(lesson.Type),
		strings.TrimSpace(lesson.Teacher),
		strings.TrimSpace(lesson.Room),
		strconv.Itoa(lesson.Subgroup),
		datePart(lesson.ValidFrom),
		datePart(lesson.ValidTo),
	}
	return strings.Join(parts, "\x1f")
}

func datePart(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func snapshotGroupRank(status string) int {
	switch status {
	case snapshotGroupChanged:
		return 0
	case snapshotGroupAdded:
		return 1
	case snapshotGroupRemoved:
		return 2
	default:
		return 3
	}
}
