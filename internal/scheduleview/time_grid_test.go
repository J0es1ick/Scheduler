package scheduleview

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestTeacherWindowIncludesUniversityBellSlot(t *testing.T) {
	day := Day{Date: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Lessons: []domain.Lesson{
		{TimeStart: "09:50", TimeEnd: "11:25", Subject: "Проектная работа", GroupName: "4/42", Room: "А306", Type: domain.LessonTypeLab},
		{TimeStart: "14:00", TimeEnd: "15:35", Subject: "Проектная работа", GroupName: "4/147", Room: "А307", Type: domain.LessonTypeLab},
	}}
	reference := []domain.LessonTimeSlot{{Start: "08:00", End: "09:35"}, {Start: "09:50", End: "11:25"}, {Start: "12:10", End: "13:45"}, {Start: "14:00", End: "15:35"}, {Start: "15:50", End: "17:25"}, {Start: "17:40", End: "19:15"}}
	slots := weekSlots([]Day{day}, false, reference)
	if len(slots) != 6 || slots[0].start != "08:00" || slots[2] != (timeSlot{start: "12:10", end: "13:45"}) || slots[4].start != "15:50" || slots[5].start != "17:40" {
		t.Fatalf("grid must include every university slot: %+v", slots)
	}
	if !isWindow(day.Lessons, slots[2], false) || isWindow(day.Lessons[:1], slots[2], false) || isWindow(day.Lessons, slots[0], false) || isWindow(day.Lessons, slots[4], false) {
		t.Fatal("incorrect window marker")
	}
	request := Request{University: "ИГХТУ", Group: "Преподаватель: Пиголицын С.М.", From: day.Date.AddDate(0, 0, -5), Days: 7, Schedule: []Day{day}, ShowGroupNames: true, TimeSlots: reference}
	for _, days := range []int{1, 7, 14} {
		request.Days = days
		if days == 1 {
			request.From = day.Date
		} else {
			request.From = day.Date.AddDate(0, 0, -5)
		}
		if days == 1 && len(dayEntries(request, day)) != 6 {
			t.Fatal("daily table omitted empty slots")
		}
		if days == 14 {
			allDays := completeDays(request)
			if nextWeek := weekSlots(allDays[7:], false, reference); !slices.Equal(slots, nextWeek) {
				t.Fatalf("empty second week has a different grid: %+v", nextWeek)
			}
		}
		if _, err := RenderPNG(request); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTimeGridIncludesAllSlotsWithOneOrNoLessons(t *testing.T) {
	reference := []domain.LessonTimeSlot{{Start: "14:00", End: "15:35"}, {Start: "08:00", End: "09:35"}, {Start: "12:10", End: "13:45"}, {Start: "09:50", End: "11:25"}, {Start: "15:50", End: "17:25"}}
	for _, lessons := range [][]domain.Lesson{nil, {{TimeStart: "12:10", TimeEnd: "13:45"}}} {
		day := Day{Date: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Lessons: lessons}
		entries := dayEntries(Request{TimeSlots: reference}, day)
		if len(entries) != 5 || entries[0].slot.start != "08:00" || entries[4].slot.start != "15:50" {
			t.Fatalf("incomplete daily grid with %d lessons: %+v", len(lessons), entries)
		}
		for index, entry := range entries {
			if (entry.lesson != nil) != (len(lessons) == 1 && index == 2) {
				t.Fatalf("lesson moved or duplicated in row %d: %+v", index, entry)
			}
		}
	}
}

func TestTimeGridSortsUnorderedLessonsBeforeFillingGaps(t *testing.T) {
	lessons := []domain.Lesson{{TimeStart: "14:00", TimeEnd: "15:35"}, {TimeStart: "09:50", TimeEnd: "11:25"}, {TimeStart: "08:00", TimeEnd: "09:35"}}
	want := []timeSlot{{start: "08:00", end: "09:35"}, {start: "09:50", end: "11:25"}, {start: "12:10", end: "13:45"}, {start: "14:00", end: "15:35"}}
	for range 100 {
		got := weekSlots([]Day{{Lessons: lessons}}, false, []domain.LessonTimeSlot{{Start: "12:10", End: "13:45"}})
		if !slices.Equal(got, want) {
			t.Fatalf("nondeterministic grid: %+v", got)
		}
	}
}

func TestUnknownTimeGridKeepsLongGapsAndCustomTimes(t *testing.T) {
	actual := []timeSlot{{start: "09:50", end: "11:25"}, {start: "14:05", end: "15:40"}}
	got := fillTimeGrid(actual, nil)
	if len(got) != 3 || got[1] != (timeSlot{start: "11:25", end: "14:05"}) || got[2] != actual[1] {
		t.Fatalf("unknown grid: %+v", got)
	}
	adjacent := fillTimeGrid([]timeSlot{{start: "09:50", end: "11:25"}, {start: "12:10", end: "13:45"}}, nil)
	if len(adjacent) != 2 {
		t.Fatalf("ordinary break became a lesson slot: %+v", adjacent)
	}
	overlapping := fillTimeGrid(actual, []domain.LessonTimeSlot{{Start: "11:00", End: "12:30"}})
	if !slices.Contains(overlapping, actual[0]) || !slices.Contains(overlapping, actual[1]) || !slices.Contains(overlapping, timeSlot{start: "11:00", end: "12:30"}) {
		t.Fatalf("overlapping reference lost a class or bell slot: %+v", overlapping)
	}
}

func TestLongLessonKeepsFullGridAndMarksOccupiedContinuation(t *testing.T) {
	reference := []domain.LessonTimeSlot{{Start: "08:00", End: "09:35"}, {Start: "09:50", End: "11:25"}, {Start: "12:10", End: "13:45"}, {Start: "14:00", End: "15:35"}, {Start: "15:50", End: "17:25"}}
	day := Day{Date: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Lessons: []domain.Lesson{{TimeStart: "08:00", TimeEnd: "17:25", Subject: "Практика"}}}
	entries := dayEntries(Request{TimeSlots: reference}, day)
	if len(entries) != 5 || entries[0].lesson == nil || entries[0].slot.end != "09:35" {
		t.Fatalf("long lesson collapsed the grid: %+v", entries)
	}
	if detail := visualLessonDetails(*entries[0].lesson, entries[0].slot); !strings.Contains(detail, "08:00–17:25") {
		t.Fatalf("actual duration lost: %s", detail)
	}
	for _, entry := range entries[1:] {
		if entry.lesson != nil || emptySlotLabel(day.Lessons, entry.slot, false) != "Продолжение занятия" {
			t.Fatalf("continuation marked as free or duplicated: %+v", entry)
		}
	}
	for _, days := range []int{1, 7} {
		if _, err := RenderPNG(Request{University: "ИГХТУ", Group: "Пример длительного занятия", From: day.Date, Days: days, Schedule: []Day{day}, TimeSlots: reference}); err != nil {
			t.Fatal(err)
		}
	}
}
