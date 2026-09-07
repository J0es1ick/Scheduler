package scheduleview

import (
	"sort"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func fillTimeGrid(occupied []timeSlot, reference []domain.LessonTimeSlot) []timeSlot {
	result := make([]timeSlot, 0, len(occupied)+len(reference))
	for _, item := range reference {
		slot := timeSlot{start: item.Start, end: item.End}
		if slot.start >= slot.end {
			continue
		}
		if _, err := time.Parse("15:04", slot.start); err != nil {
			continue
		}
		if _, err := time.Parse("15:04", slot.end); err != nil {
			continue
		}
		overlap := false
		for _, existing := range result {
			if slot.start < existing.end && existing.start < slot.end {
				overlap = true
				break
			}
		}
		if !overlap {
			result = append(result, slot)
		}
	}
	referenceCount := len(result)
	for _, slot := range occupied {
		found := false
		for index, existing := range result {
			if slot.start != existing.start {
				continue
			}
			if index >= referenceCount && slot.end > existing.end {
				result[index].end = slot.end
			}
			found = true
			break
		}
		if !found {
			result = append(result, slot)
		}
	}
	if len(result) == 0 {
		return result
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].start == result[j].start {
			return result[i].end < result[j].end
		}
		return result[i].start < result[j].start
	})
	complete := make([]timeSlot, 0, len(result)*2)
	end := result[0].end
	for i, slot := range result {
		if i > 0 && slot.start > end {
			a, ae := time.Parse("15:04", end)
			b, be := time.Parse("15:04", slot.start)
			if ae == nil && be == nil && b.Sub(a) >= time.Hour {
				complete = append(complete, timeSlot{start: end, end: slot.start})
			}
		}
		complete = append(complete, slot)
		if slot.end > end {
			end = slot.end
		}
	}
	return complete
}

func emptySlotLabel(lessons []domain.Lesson, slot timeSlot, normalize bool) string {
	for _, lesson := range lessons {
		value := visualTimeSlot(lesson, normalize)
		if value.start < slot.start && value.end > slot.start {
			return "Продолжение занятия"
		}
	}
	if isWindow(lessons, slot, normalize) {
		return "Окно"
	}
	return ""
}

func isWindow(lessons []domain.Lesson, slot timeSlot, normalize bool) bool {
	before, after := false, false
	for _, lesson := range lessons {
		value := visualTimeSlot(lesson, normalize)
		if value.end <= slot.start {
			before = true
		}
		if value.start >= slot.end {
			after = true
		}
		if value.start < slot.end && slot.start < value.end {
			return false
		}
	}
	return before && after
}

type dayEntry struct {
	slot   timeSlot
	lesson *domain.Lesson
}

func dayEntries(request Request, day Day) []dayEntry {
	rows := []dayEntry{}
	for _, slot := range weekSlots([]Day{day}, false, request.TimeSlots) {
		lessons := lessonsAt(day.Lessons, slot, false)
		if len(lessons) == 0 {
			rows = append(rows, dayEntry{slot: slot})
			continue
		}
		for _, lesson := range lessons {
			rows = append(rows, dayEntry{slot: slot, lesson: &lesson})
		}
	}
	return rows
}
