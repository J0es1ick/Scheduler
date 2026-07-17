package isuct

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestParseAutocompleteResponseLiveFormat(t *testing.T) {
	body := []byte("\xEF\xBB\xBF" + `{"1/0|21299":"1/0","3/147|12345":"3/147"}`)
	items, err := parseAutocompleteResponse(body)
	if err != nil {
		t.Fatalf("parseAutocompleteResponse() error = %v", err)
	}
	got := make(map[string]string, len(items))
	for _, item := range items {
		got[item.Value] = item.Label
	}
	if got["21299"] != "1/0" || got["12345"] != "3/147" {
		t.Fatalf("unexpected groups: %#v", got)
	}
}

func TestParseAJAXScheduleFragment(t *testing.T) {
	body := []byte(`[{"command":"settings","data":""},{"command":"insert","data":"<div id=\"form-ajax-node-content\"><table class=\"schedule\"></table></div>"}]`)
	fragment, err := parseAJAXScheduleFragment(body)
	if err != nil {
		t.Fatalf("parseAJAXScheduleFragment() error = %v", err)
	}
	if fragment == "" {
		t.Fatal("expected schedule HTML fragment")
	}
}

func TestParseScheduleTableLiveStructure(t *testing.T) {
	html := `<table class="schedule">
		<tr><td rowspan="2" class="week">нед</td><td rowspan="2" class="time">Время</td><td colspan="6">Занятия</td></tr>
		<tr><td>Пн</td><td>Вт</td><td>Ср</td><td>Чт</td><td>Пт</td><td>Сб</td></tr>
		<tr><td rowspan="1">1</td><td class="time">09:50-11:25</td><td>&nbsp;</td><td class="type-pz">Иностранный язык Кузьмина Р.В. пр.з. К409 <br />с 10.02.2026 по 16.06.2026</td><td>&nbsp;</td><td>&nbsp;</td><td>&nbsp;</td><td>&nbsp;</td></tr>
		<tr><td rowspan="1">2</td><td class="time">12:10-13:45</td><td class="type-lk">Математика Иванов И.И. лк. Г101 <br />с 02.02.2026 по 25.05.2026</td><td>&nbsp;</td><td>&nbsp;</td><td>&nbsp;</td><td>&nbsp;</td><td>&nbsp;</td></tr>
	</table>`

	lessons, err := parseScheduleTable(html, "isuct:group:21299", "isuct-current")
	if err != nil {
		t.Fatalf("parseScheduleTable() error = %v", err)
	}
	if len(lessons) != 2 {
		t.Fatalf("len(lessons) = %d, want 2: %#v", len(lessons), lessons)
	}
	first := lessons[0]
	if first.DayOfWeek != 2 || first.WeekType != domain.WeekTypeOdd {
		t.Fatalf("unexpected first lesson day/week: %d/%s", first.DayOfWeek, first.WeekType)
	}
	if first.Subject != "Иностранный язык" || first.Teacher != "Кузьмина Р.В." || first.Room != "К409" {
		t.Fatalf("unexpected parsed lesson: %#v", first)
	}
	if first.ValidFrom == nil || first.ValidFrom.Format("2006-01-02") != "2026-02-10" {
		t.Fatalf("unexpected ValidFrom: %v", first.ValidFrom)
	}
	if lessons[1].DayOfWeek != 1 || lessons[1].WeekType != domain.WeekTypeEven {
		t.Fatalf("unexpected second lesson day/week: %d/%s", lessons[1].DayOfWeek, lessons[1].WeekType)
	}
}

func TestLiveISUCTAdapter(t *testing.T) {
	if os.Getenv("ISUCT_INTEGRATION_TEST") != "1" {
		t.Skip("set ISUCT_INTEGRATION_TEST=1 to call the live ISUCT website")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	adapter := New("isuct-current")
	groups, err := adapter.FetchGroups(ctx)
	if err != nil {
		t.Fatalf("FetchGroups() error = %v", err)
	}
	if len(groups) < 50 {
		t.Fatalf("FetchGroups() returned only %d groups", len(groups))
	}

	var selected *domain.Group
	for i := range groups {
		if groups[i].Name == "1/0" {
			selected = &groups[i]
			break
		}
	}
	if selected == nil {
		t.Fatal("live group 1/0 was not discovered")
	}
	lessons, err := adapter.FetchSchedule(ctx, selected.ID)
	if err != nil {
		t.Fatalf("FetchSchedule(%s) error = %v", selected.Name, err)
	}
	if len(lessons) == 0 {
		t.Fatalf("FetchSchedule(%s) returned no lessons", selected.Name)
	}
	t.Logf("live ISUCT: %d groups, %d lessons for %s", len(groups), len(lessons), selected.Name)
}
