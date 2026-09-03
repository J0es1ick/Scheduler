package service

import (
	"context"
	"sort"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/searchtext"
)

type teacherMatch struct {
	name  string
	score int
}

func (s *ScheduleService) FindTeachers(
	ctx context.Context,
	universityID string,
	query string,
) ([]string, error) {
	stored, err := s.lessonRepo.GetTeacherNames(ctx, universityID)
	if err != nil {
		return nil, err
	}
	names := uniqueTeacherNames(stored)
	if len(searchtext.Tokens(query)) == 0 {
		sort.Slice(names, func(i, j int) bool {
			return searchtext.TokenKey(names[i]) < searchtext.TokenKey(names[j])
		})
		return names, nil
	}
	matches := make([]teacherMatch, 0, len(names))
	for _, name := range names {
		score, ok := searchtext.MatchTeacher(name, query)
		if ok {
			matches = append(matches, teacherMatch{name: name, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return searchtext.TokenKey(matches[i].name) < searchtext.TokenKey(matches[j].name)
	})
	if len(matches) > 12 {
		matches = matches[:12]
	}
	result := make([]string, len(matches))
	for index := range matches {
		result[index] = matches[index].name
	}
	return result, nil
}

func uniqueTeacherNames(stored []string) []string {
	seen := make(map[string]struct{}, len(stored))
	result := make([]string, 0, len(stored))
	for _, value := range stored {
		parts := strings.FieldsFunc(value, func(character rune) bool {
			return character == ';' || character == '\n' || character == '\r' || character == '|'
		})
		for _, part := range parts {
			name := strings.TrimSpace(part)
			normalized := searchtext.TokenKey(name)
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}
