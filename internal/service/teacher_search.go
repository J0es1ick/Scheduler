package service

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
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
	queryTokens := teacherTokens(query)
	if len(queryTokens) == 0 {
		sort.Slice(names, func(i, j int) bool {
			return normalizedTeacherName(names[i]) < normalizedTeacherName(names[j])
		})
		return names, nil
	}
	matches := make([]teacherMatch, 0, len(names))
	for _, name := range names {
		score, ok := teacherMatchScore(teacherTokens(name), queryTokens)
		if ok {
			matches = append(matches, teacherMatch{name: name, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score < matches[j].score
		}
		return normalizedTeacherName(matches[i].name) < normalizedTeacherName(matches[j].name)
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
			normalized := normalizedTeacherName(name)
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

func teacherTokens(value string) []string {
	value = strings.ToLower(strings.ReplaceAll(value, "ё", "е"))
	value = strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return character
		}
		return ' '
	}, value)
	return strings.Fields(value)
}

func normalizedTeacherName(value string) string {
	return strings.Join(teacherTokens(value), " ")
}

func teacherMatchScore(candidate, query []string) (int, bool) {
	if len(candidate) == 0 || len(query) == 0 || len(query) > len(candidate) {
		return 0, false
	}
	used := make([]bool, len(candidate))
	total := 0
	for _, queryToken := range query {
		bestIndex := -1
		bestScore := 1000
		for index, candidateToken := range candidate {
			if used[index] {
				continue
			}
			score, ok := teacherTokenScore(candidateToken, queryToken)
			if ok && score < bestScore {
				bestIndex = index
				bestScore = score
			}
		}
		if bestIndex < 0 {
			return 0, false
		}
		used[bestIndex] = true
		total += bestScore
	}
	if strings.Join(candidate, " ") == strings.Join(query, " ") {
		return 0, true
	}
	return total + len(candidate) - len(query), true
}

func teacherTokenScore(candidate, query string) (int, bool) {
	if candidate == query {
		return 0, true
	}
	queryLength := utf8.RuneCountInString(query)
	if queryLength >= 2 && strings.HasPrefix(candidate, query) {
		return 2, true
	}
	if queryLength >= 4 && oneEditApart(candidate, query) {
		return 5, true
	}
	return 0, false
}

func oneEditApart(left, right string) bool {
	a, b := []rune(left), []rune(right)
	if len(a) > len(b)+1 || len(b) > len(a)+1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		switch {
		case len(a) > len(b):
			i++
		case len(b) > len(a):
			j++
		case i+1 < len(a) && j+1 < len(b) && a[i] == b[j+1] && a[i+1] == b[j]:
			i += 2
			j += 2
		default:
			i++
			j++
		}
	}
	return edits+len(a)-i+len(b)-j <= 1
}
