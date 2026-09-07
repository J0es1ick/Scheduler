package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/scheduleview"
	"github.com/J0es1ick/Scheduler/internal/service"
)

func (s *Server) handleEditorCalendar(w http.ResponseWriter, r *http.Request) {
	schedule, err := s.store.EditorSchedule(r.Context(), r.URL.Query().Get("group"))
	if err != nil {
		writeEditorError(w, err)
		return
	}
	location, err := time.LoadLocation(schedule.Group.Timezone)
	if err != nil {
		writeEditorError(w, err)
		return
	}
	from, err := time.ParseInLocation(time.DateOnly, r.URL.Query().Get("from"), location)
	days, dayErr := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || dayErr != nil || days < 1 || days > 112 {
		writeAPIError(w, 400, "Выберите диапазон от 1 до 112 дней")
		return
	}
	request := scheduleview.Request{University: schedule.Group.UniversityName, Group: schedule.Group.Name, From: from, Days: days}
	starts := map[string]time.Time{}
	for _, semester := range schedule.Semesters {
		starts[semester.ID] = semester.StartDate
	}
	for i := 0; i < days; i++ {
		day := scheduleview.Day{Date: from.AddDate(0, 0, i)}
		for _, lesson := range schedule.Lessons {
			value := domain.Lesson{ID: lesson.ID, GroupID: lesson.GroupID, Subject: lesson.Subject, Type: domain.LessonType(lesson.Type), Teacher: lesson.Teacher, Room: lesson.Room, Subgroup: lesson.Subgroup, TimeStart: lesson.TimeStart, TimeEnd: lesson.TimeEnd, DayOfWeek: lesson.DayOfWeek, WeekType: domain.WeekType(lesson.WeekType), SpecialDate: lesson.SpecialDate, ValidFrom: lesson.ValidFrom, ValidTo: lesson.ValidTo, Recurrence: lesson.Recurrence}
			start := starts[lesson.SemesterID]
			if service.LessonMatchesDate(value, day.Date, &start) {
				day.Lessons = append(day.Lessons, value)
			}
		}
		request.Schedule = append(request.Schedule, day)
	}
	content, err := scheduleview.RenderICS(request)
	if err != nil {
		writeEditorError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"content": string(content)})
}
