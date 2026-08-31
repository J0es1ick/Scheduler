package service

import (
	"slices"
	"testing"
)

func TestTeacherMatchingSupportsPartialNamesAndInitials(t *testing.T) {
	names := uniqueTeacherNames([]string{
		"Константинов Е.С.",
		"Константинов А.В.",
		"Сизова О.В.; Петров П.П.",
		"Сизова О.В.",
	})
	for _, test := range []struct {
		query string
		want  []string
	}{
		{query: "Конст", want: []string{"Константинов А.В.", "Константинов Е.С."}},
		{query: "Константинов Е С", want: []string{"Константинов Е.С."}},
		{query: "Сизова", want: []string{"Сизова О.В."}},
		{query: "О В", want: []string{"Сизова О.В."}},
		{query: "Канстантинов Е.С.", want: []string{"Константинов Е.С."}},
	} {
		matches := make([]teacherMatch, 0)
		for _, name := range names {
			if score, ok := teacherMatchScore(teacherTokens(name), teacherTokens(test.query)); ok {
				matches = append(matches, teacherMatch{name: name, score: score})
			}
		}
		got := make([]string, 0, len(matches))
		for _, match := range matches {
			got = append(got, match.name)
		}
		slices.Sort(got)
		slices.Sort(test.want)
		if !slices.Equal(got, test.want) {
			t.Fatalf("query %q matched %#v, want %#v", test.query, got, test.want)
		}
	}
}
