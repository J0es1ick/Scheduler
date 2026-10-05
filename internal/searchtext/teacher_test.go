package searchtext

import "testing"

func TestTeacherIdentityAndQueries(t *testing.T) {
	names := TeacherNames([]string{"Сизёва О.В., Иванов Иван Иванович; Петров П.П.|Сизева О В\nПетров П.П."})
	if len(names) != 3 {
		t.Fatalf("names %#v", names)
	}
	for _, query := range []string{"сизева", "сизёва о в", "Сизев", "Сизева Ольга Владимировна"} {
		if _, ok := MatchTeacher(names[0], query); !ok {
			t.Errorf("query %q did not match", query)
		}
	}
	if _, ok := MatchTeacher("Иванов Иван Иванович", "Иванов И.И."); !ok {
		t.Fatal("initials failed")
	}
	if !HasTeacher("Сизёва О.В., Петров П.П.", "Сизева О В") {
		t.Fatal("punctuation changed identity")
	}
	if HasTeacher("Иванова А.А.", "Иванов А.А.") || HasTeacher("Иванов А.А.", "Иванов Б.Б.") {
		t.Fatal("different teachers merged")
	}
}
