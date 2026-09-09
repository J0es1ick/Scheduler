package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type teacherScheduleScenarioService struct {
	*service.ScheduleService
	teachers []string
	queries  []string
}

func (s *teacherScheduleScenarioService) FindTeachers(_ context.Context, _ string, query string) ([]string, error) {
	if query == "" {
		return append([]string(nil), s.teachers...), nil
	}
	result := make([]string, 0)
	for _, teacher := range s.teachers {
		if strings.Contains(strings.ToLower(teacher), strings.ToLower(query)) || strings.HasPrefix(strings.ToLower(teacher), strings.ToLower(query)) {
			result = append(result, teacher)
		}
	}
	return result, nil
}

func (s *teacherScheduleScenarioService) GetScheduleForTeacherRange(
	_ context.Context,
	_ string,
	teacher string,
	from time.Time,
	_ time.Time,
) (map[time.Time][]domain.Lesson, error) {
	s.queries = append(s.queries, teacher)
	return map[time.Time][]domain.Lesson{dateAtLocation(from, from.Location()): {{
		Subject: "Проектирование", TimeStart: "09:50", TimeEnd: "11:25",
		Type: domain.LessonTypeLab, Teacher: teacher, Room: "А208", GroupName: "3/42",
	}}}, nil
}

func TestTeacherSearchSelectsCandidateAndKeepsFullNavigation(t *testing.T) {
	scenario := newTelegramScenario(t)
	schedule := &teacherScheduleScenarioService{teachers: []string{"Константинов А.В.", "Константинов Е.С."}}
	handler := &Handler{
		ScheduleService: schedule,
		StateManager:    state.NewManager(),
		UserService: &navigationUserService{user: domain.User{
			ID: "42", DefaultGroupID: "primary", SearchScheduleView: domain.ScheduleViewVisual,
		}},
		GroupService: &navigationGroupService{group: domain.Group{
			ID: "primary", UniversityID: "isuct", Name: "4/147", IsActive: true,
		}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{
			"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true, Timezone: "Europe/Moscow"},
		}},
	}
	current := &dto.UserState{
		UniversityID: "isuct", University: "ИГХТУ", GroupID: "primary", Query: "4/147", GroupActive: true, Step: "done",
	}
	contextValue := scenario.bot.NewContext(tele.Update{Message: &tele.Message{
		Text: "Константинов", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
	}})
	ctx, cancel := reqCtx()
	defer cancel()
	if err := handler.beginTeacherSearch(ctx, contextValue, current, "Константинов", "quick"); err != nil {
		t.Fatal(err)
	}
	stored := handler.StateManager.Get(42)
	if stored == nil || stored.Step != "choosing_teacher" || len(stored.TeacherCandidates) != 2 {
		t.Fatalf("teacher selection state = %#v", stored)
	}
	scenario.callback(t, handler.HandleTeacherSelect, scenario.button(t, "Константинов Е.С."), false)
	scenario.requireActions(t,
		"schedule_week", "open_calendar", "open_schedule_exports", "search_teacher_again", "open_main_menu",
	)
	scenario.callback(t, handler.HandleScheduleWeekSelect, scenario.button(t, "Две недели"), true)
	if len(schedule.queries) != 2 || schedule.queries[0] != "Константинов Е.С." || schedule.queries[1] != "Константинов Е.С." {
		t.Fatalf("teacher schedule queries = %#v", schedule.queries)
	}
	scenario.mu.Lock()
	methods := append([]string(nil), scenario.methods...)
	scenario.mu.Unlock()
	if !slicesContain(methods, "sendPhoto") {
		t.Fatalf("default teacher view did not render a table: %#v", methods)
	}
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
