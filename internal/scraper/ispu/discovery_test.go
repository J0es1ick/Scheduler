package ispu

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverySeparatesReusedGroupIDsByFacultyAndCourse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		selected := func(name, fallback string) string {
			if value := r.FormValue(name); value != "" {
				return value
			}
			return fallback
		}
		schedule := selected(scheduleControl, "143")
		faculty := selected(facultyControl, "30000")
		course := selected(courseControl, "1")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<form id="form1"><input type="hidden" name="__VIEWSTATE" value="state"><input type="hidden" name="__VIEWSTATEGENERATOR" value="generator"><input type="hidden" name="__EVENTVALIDATION" value="validation">`)
		writeSelect := func(control, value string, items ...string) {
			fmt.Fprintf(w, `<select name="%s">`, html.EscapeString(control))
			for i := 0; i < len(items); i += 2 {
				attribute := ""
				if items[i] == value {
					attribute = ` selected="selected"`
				}
				fmt.Fprintf(w, `<option value="%s"%s>%s</option>`, items[i], attribute, items[i+1])
			}
			fmt.Fprint(w, `</select>`)
		}
		writeSelect(scheduleControl, schedule, "143", "лекционное", "144", "практическое")
		writeSelect(facultyControl, faculty, "30000", "ИВТФ", "40000", "ТЭФ")
		writeSelect(courseControl, course, "1", "1", "2", "2")
		writeSelect(groupControl, "101032", "101032", "40")
		fmt.Fprint(w, weeklyTable("начало:01.09.2026 вторник 2 недели - окончание:14.09.2026", `<tr><td>2</td><td>8.00-9.35</td><td></td><td>Предмет `+faculty+`/`+course+`/`+schedule+` лек.</td><td></td><td></td><td></td><td></td><td></td></tr>`))
		fmt.Fprint(w, `</form>`)
	}))
	defer server.Close()
	adapter := newAdapter(server.URL, "semester", server.Client())
	groups, err := adapter.FetchGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 4 {
		t.Fatalf("four groups with the same source option ID were merged: %#v", groups)
	}
	for _, group := range groups {
		key := extractExtID(group.ID)
		parts := strings.Split(key, ":")
		if len(parts) != 3 {
			t.Fatalf("unscoped source identity: %s", group.ID)
		}
		if !strings.HasPrefix(group.Name, parts[1]+"-40 (") {
			t.Fatalf("wrong course or missing faculty in group name: %#v", group)
		}
		if len(adapter.groups[key].paths) != 2 {
			t.Fatalf("same group should retain both schedule types: %#v", adapter.groups[key])
		}
		lessons, err := adapter.FetchSchedule(context.Background(), group.ID)
		if err != nil || len(lessons) != 2 {
			t.Fatalf("group %s: lessons=%#v err=%v", group.Name, lessons, err)
		}
		for _, lesson := range lessons {
			if !strings.HasPrefix(lesson.Subject, "Предмет "+parts[0]+"/"+parts[1]+"/") {
				t.Fatalf("lesson from a different faculty or course assigned to %s: %#v", group.Name, lesson)
			}
		}
	}
}
