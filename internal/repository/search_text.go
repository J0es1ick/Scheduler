package repository

import (
	"strings"
	"unicode/utf8"
)

func normalizedSearch(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func closeSearchMatch(value, query string) bool {
	value, query = normalizedSearch(value), normalizedSearch(query)
	if utf8.RuneCountInString(query) < 3 || utf8.RuneCountInString(query) > 80 {
		return false
	}
	a, b := []rune(value), []rune(query)
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
