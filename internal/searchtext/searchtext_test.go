package searchtext

import "testing"

func TestNormalize(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "  Ёж\u00a0ИВАНОВ  ", want: "еж иванов"},
		{input: "А-101\tкорпус", want: "а-101 корпус"},
	} {
		if got := Normalize(test.input); got != test.want {
			t.Fatalf("Normalize(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestEntityPolicies(t *testing.T) {
	for _, test := range []struct {
		name      string
		candidate string
		query     string
		match     func(string, string) bool
		want      bool
	}{
		{name: "teacher initials", candidate: "Сизёва О.В.", query: "сизева о в", match: scored(MatchTeacher), want: true},
		{name: "teacher typo", candidate: "Константинов Е.С.", query: "канстантинов", match: scored(MatchTeacher), want: true},
		{name: "discipline tokens", candidate: "Инструментальные средства информационных систем", query: "информ систем", match: scored(MatchDiscipline), want: true},
		{name: "discipline typo", candidate: "Высшая математика", query: "матиматика", match: scored(MatchDiscipline), want: true},
		{name: "room whitespace", candidate: "  А-101   корпус 1", query: "а-101 корпус", match: MatchRoom, want: true},
		{name: "room no typo", candidate: "А-201", query: "А-202", match: MatchRoom, want: false},
		{name: "group typo", candidate: "4/147", query: "4/174", match: CloseIdentifier, want: true},
		{name: "short group no typo", candidate: "АБ", query: "АВ", match: CloseIdentifier, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.match(test.candidate, test.query); got != test.want {
				t.Fatalf("match(%q, %q) = %t, want %t", test.candidate, test.query, got, test.want)
			}
		})
	}
}

func scored(match func(string, string) (int, bool)) func(string, string) bool {
	return func(candidate, query string) bool {
		_, ok := match(candidate, query)
		return ok
	}
}
