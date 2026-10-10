package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

type PersonalPattern struct {
	SchedulePattern
	GroupID    string `json:"group_id"`
	GroupName  string `json:"group_name"`
	SemesterID string `json:"semester_id"`
}

func (s *ScheduleService) targetLessons(ctx context.Context, target domain.PersonalTarget) ([]domain.Lesson, error) {
	if target.Role == domain.RoleTeacher {
		return s.lessonRepo.GetLessonsByTeacher(ctx, target.UniversityID, target.Name)
	}
	return s.lessonRepo.GetLessonsByGroupID(ctx, target.ID)
}

func (s *ScheduleService) personalPatterns(ctx context.Context, target domain.PersonalTarget, from, to time.Time) ([]PersonalPattern, error) {
	lessons, err := s.targetLessons(ctx, target)
	if err != nil {
		return nil, err
	}
	groups := map[string]bool{}
	for _, lesson := range lessons {
		groups[lesson.GroupID] = true
	}
	result := []PersonalPattern{}
	for groupID := range groups {
		groupLessons, err := s.lessonRepo.GetLessonsByGroupID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		semesters, err := s.buildSemesterCacheForLessons(ctx, groupLessons)
		if err != nil {
			return nil, err
		}
		for _, semester := range semesters {
			if semester.EndDate.Before(from) || semester.StartDate.After(to) {
				continue
			}
			pattern := PersonalPattern{SchedulePattern: AnalyzeSchedulePattern(groupLessons, *semester), GroupID: groupID, SemesterID: semester.ID}
			group, err := s.groupRepo.GetGroupByID(ctx, groupID)
			if err != nil {
				return nil, err
			}
			if group != nil {
				pattern.GroupName = group.Name
			}
			result = append(result, pattern)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GroupID == result[j].GroupID {
			return result[i].SemesterID < result[j].SemesterID
		}
		return result[i].GroupID < result[j].GroupID
	})
	return result, nil
}

type PersonalReview struct {
	Items        []PersonalReviewItem      `json:"items"`
	Kept         int                       `json:"kept"`
	Dropped      int                       `json:"dropped"`
	Replacements []domain.PersonalOverride `json:"-"`
}

type PersonalReviewItem struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Subject string `json:"subject"`
	Kept    int    `json:"kept"`
	Dropped int    `json:"dropped"`
}

func samePersonalSlot(basis, current domain.Lesson, date time.Time) bool {
	day := basis.DayOfWeek
	if basis.SpecialDate != nil {
		day = helpers.Weekday(*basis.SpecialDate)
	}
	return basis.GroupID == current.GroupID && basis.SemesterID == current.SemesterID && day == helpers.Weekday(date) && basis.TimeStart == current.TimeStart && basis.TimeEnd == current.TimeEnd && basis.Subgroup == current.Subgroup
}

func remapPersonalChange(change domain.PersonalOverride, lessons []domain.Lesson, semester domain.Semester) (domain.PersonalOverride, PersonalReviewItem, error) {
	item := PersonalReviewItem{ID: change.ID, Version: change.Version}
	var basis domain.Lesson
	if err := json.Unmarshal(change.Basis, &basis); err != nil {
		return change, item, err
	}
	item.Subject = basis.Subject
	var old []domain.PersonalOccurrence
	if err := json.Unmarshal(change.Occurrences, &old); err != nil {
		return change, item, err
	}
	if len(old) == 0 {
		for day := change.ValidFrom; !day.After(change.ValidTo) && !day.After(change.ValidFrom.AddDate(1, 0, 0)); day = day.AddDate(0, 0, 1) {
			if change.Scope == "day" || LessonMatchesDate(basis, day, &semester.StartDate) {
				old = append(old, domain.PersonalOccurrence{Date: day.Format(time.DateOnly)})
			}
		}
	}
	replacement := []domain.PersonalOccurrence{}
	for _, occurrence := range old {
		date, err := time.Parse(time.DateOnly, occurrence.Date)
		if err != nil {
			return change, item, err
		}
		candidates := []domain.Lesson{}
		for _, lesson := range lessons {
			if !date.Before(semester.StartDate) && !date.After(semester.EndDate) && samePersonalSlot(basis, lesson, date) && LessonMatchesDate(lesson, date, &semester.StartDate) {
				candidates = append(candidates, lesson)
			}
		}
		if len(candidates) != 1 {
			item.Dropped++
			continue
		}
		id := candidates[0].PersonalKey
		if id == "" {
			id = candidates[0].ID
		}
		replacement = append(replacement, domain.PersonalOccurrence{LessonID: id, Date: occurrence.Date})
		item.Kept++
	}
	if len(old) == 0 {
		item.Dropped = 1
	}
	var err error
	change.Occurrences, err = json.Marshal(replacement)
	return change, item, err
}

func (s *ScheduleService) personalReview(ctx context.Context, target domain.PersonalTarget, changes []domain.PersonalOverride) (*PersonalReview, error) {
	var result *PersonalReview
	var lessons []domain.Lesson
	for _, change := range changes {
		if !change.NeedsReview {
			continue
		}
		if result == nil {
			result = &PersonalReview{Items: []PersonalReviewItem{}}
			var err error
			lessons, err = s.targetLessons(ctx, target)
			if err != nil {
				return nil, err
			}
		}
		semester, err := s.personalRepo.Semester(ctx, change.SemesterID)
		if err != nil {
			return nil, err
		}
		replacement, item, err := remapPersonalChange(change, lessons, *semester)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
		result.Replacements = append(result.Replacements, replacement)
		result.Kept += item.Kept
		result.Dropped += item.Dropped
	}
	return result, nil
}

type PersonalReviewInput struct {
	TargetID    string           `json:"target_id"`
	Publication string           `json:"publication"`
	Action      string           `json:"action"`
	Versions    map[string]int64 `json:"versions"`
}

func (s *ScheduleService) ResolvePersonalReview(ctx context.Context, userID string, input PersonalReviewInput) (*PersonalReview, error) {
	if input.Action != "keep" && input.Action != "discard" {
		return nil, personalInput("Выберите, сохранить или сбросить правки")
	}
	date := helpers.NormalizeDate(time.Now())
	schedule, err := s.PersonalSchedule(ctx, userID, input.TargetID, date, date)
	if err != nil {
		return nil, err
	}
	if input.Publication != schedule.Publication || schedule.Review == nil || len(input.Versions) != len(schedule.Review.Items) {
		return nil, repository.ErrPersonalConflict
	}
	for i, item := range schedule.Review.Items {
		if input.Versions[item.ID] != item.Version {
			return nil, repository.ErrPersonalConflict
		}
		if input.Action == "discard" {
			schedule.Review.Replacements[i].Occurrences = []byte(`[]`)
		}
	}
	err = s.personalRepo.ResolveReview(ctx, userID, schedule.Target, schedule.Publication, schedule.Review.Replacements)
	return schedule.Review, err
}
