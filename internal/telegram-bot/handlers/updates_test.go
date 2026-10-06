package handlers

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
)

type updatesProfileFake struct{ *roleProfileFake }

func (f *updatesProfileFake) SetServiceUpdates(_ context.Context, _ string, enabled bool, key string) (bool, error) {
	if key != "" && (key != f.user.ServiceUpdatesPromptKey || f.user.ServiceUpdatesConsent != nil) {
		return false, nil
	}
	f.user.ServiceUpdatesConsent = &enabled
	return true, nil
}
func (f *updatesProfileFake) MarkServiceUpdatesPrompt(context.Context, string) error {
	now := time.Now()
	f.user.ServiceUpdatesPromptDeliveredAt = &now
	return nil
}
func TestUpdatesOnboardingBothRolesAndDurableButtons(t *testing.T) {
	for _, role := range []domain.UserRole{domain.RoleStudent, domain.RoleTeacher} {
		for _, yes := range []bool{false, true} {
			t.Run(string(role)+map[bool]string{true: "yes", false: "no"}[yes], func(t *testing.T) {
				scenario := newTelegramScenario(t)
				profile := &updatesProfileFake{&roleProfileFake{navigationUserService: &navigationUserService{user: domain.User{ID: "42", Role: role, DefaultGroupID: "g", DailySetup: "done", ServiceUpdatesPromptKey: "persistent"}}, teacher: &domain.Teacher{ID: "t", UniversityID: "u", Name: "Иванов И.И."}}}
				if role == domain.RoleTeacher {
					profile.user.TeacherID = "t"
				}
				groups := &navigationGroupService{group: domain.Group{ID: "g", Name: "Group", UniversityID: "u", IsActive: true}}
				h := &Handler{SubscriptionService: &subscriptionScenarioService{}, UserService: profile, ProfileService: profile, UpdatesService: profile, StateManager: state.NewManager(), GroupService: groups, ScheduleService: &scheduleScenarioService{empty: true}, UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "Вуз", Timezone: "Europe/Moscow", IsActive: true}}}}
				if role == domain.RoleTeacher {
					h.ScheduleService = &teacherScheduleScenarioService{teachers: []string{"Иванов И.И."}}
				}
				if err := h.finishProfileSetup(roleTestContext(scenario, "")); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(scenario.text, "об обновлениях сервиса") || profile.user.ServiceUpdatesPromptDeliveredAt == nil {
					t.Fatalf("offer %s", scenario.text)
				}
				button := scenario.button(t, map[bool]string{true: "Да", false: "Нет"}[yes])
				h.StateManager = state.NewManager()
				scenario.callback(t, h.HandleUpdatesChoice, button, false)
				if profile.user.ServiceUpdatesConsent == nil || *profile.user.ServiceUpdatesConsent != yes {
					t.Fatal("consent not stored")
				}
				scenario.callback(t, h.HandleUpdatesChoice, button, false)
				if *profile.user.ServiceUpdatesConsent != yes {
					t.Fatal("repeated button changed consent")
				}
				if err := h.HandleUpdates(roleTestContext(scenario, "/updates off")); err != nil {
					t.Fatal(err)
				}
				if *profile.user.ServiceUpdatesConsent {
					t.Fatal("unsubscribe failed")
				}
				profile.user.ServiceUpdatesBackfill = true
				profile.user.ServiceUpdatesConsent = nil
				if err := h.finishProfileSetup(roleTestContext(scenario, "")); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(scenario.text, "об обновлениях сервиса") {
					t.Fatal("existing user received repeated onboarding prompt")
				}
			})
		}
	}
}
