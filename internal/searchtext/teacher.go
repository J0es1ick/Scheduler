package searchtext

import "strings"

func TeacherNames(values []string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '|' || r == '/'
		}) {
			name := strings.Join(strings.Fields(part), " ")
			key := TokenKey(name)
			if key != "" && !seen[key] {
				seen[key] = true
				names = append(names, name)
			}
		}
	}
	return names
}

func HasTeacher(value, name string) bool {
	key := TokenKey(name)
	for _, candidate := range TeacherNames([]string{value}) {
		if TokenKey(candidate) == key {
			return true
		}
	}
	return false
}
