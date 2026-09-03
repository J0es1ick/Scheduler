package searchtext

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func Normalize(value string) string {
	value = strings.ReplaceAll(strings.ToLower(value), "ё", "е")
	return strings.Join(strings.Fields(value), " ")
}

func Tokens(value string) []string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return character
		}
		return ' '
	}, Normalize(value))
	return strings.Fields(value)
}

func TokenKey(value string) string {
	return strings.Join(Tokens(value), " ")
}

func MatchTeacher(candidate, query string) (int, bool) {
	return matchTokens(Tokens(candidate), Tokens(query))
}

func MatchDiscipline(candidate, query string) (int, bool) {
	return matchTokens(Tokens(candidate), Tokens(query))
}

func MatchRoom(candidate, query string) bool {
	candidate, query = Normalize(candidate), Normalize(query)
	return query != "" && utf8.RuneCountInString(query) <= 80 && strings.Contains(candidate, query)
}

func CloseIdentifier(candidate, query string) bool {
	candidate, query = Normalize(candidate), Normalize(query)
	if utf8.RuneCountInString(query) < 3 || utf8.RuneCountInString(query) > 80 {
		return false
	}
	return withinOneEdit(candidate, query)
}

func matchTokens(candidate, query []string) (int, bool) {
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
			score, ok := tokenScore(candidateToken, queryToken)
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

func tokenScore(candidate, query string) (int, bool) {
	if candidate == query {
		return 0, true
	}
	queryLength := utf8.RuneCountInString(query)
	if queryLength >= 2 && strings.HasPrefix(candidate, query) {
		return 2, true
	}
	if queryLength >= 4 && withinOneEdit(candidate, query) {
		return 5, true
	}
	return 0, false
}

func withinOneEdit(left, right string) bool {
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
