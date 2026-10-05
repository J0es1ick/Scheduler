package handlers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/searchtext"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type roleProfileFake struct {
	*navigationUserService
	teacher *domain.Teacher
	saves   int
}

func (f *roleProfileFake) GetTeacher(context.Context, string) (*domain.Teacher, error) {
	return f.teacher, nil
}
func (f *roleProfileFake) SetTeacher(_ context.Context, _ string, university, name string) error {
	f.teacher = &domain.Teacher{ID: "teacher", UniversityID: university, Name: name, NameKey: searchtext.TokenKey(name)}
	f.user.Role = domain.RoleTeacher
	f.user.TeacherID = f.teacher.ID
	f.saves++
	return nil
}
func (f *roleProfileFake) SetRole(_ context.Context, _ string, role domain.UserRole) error {
	f.user.Role = role
	return nil
}
func (f *roleProfileFake) SetTeacherView(_ context.Context, _ string, view domain.ScheduleViewFormat) error {
	f.user.TeacherScheduleView = view
	return nil
}
func (f *roleProfileFake) SetDailySetup(_ context.Context, _ string, step string) error {
	if f.user.DailySetup != "done" {
		f.user.DailySetup = step
	}
	return nil
}
func (f *roleProfileFake) SetDailySchedule(_ context.Context, _ string, enabled bool, clock string) error {
	parsed, err := service.ParseDailyTime(clock)
	if err != nil {
		return err
	}
	f.user.DailyEnabled = enabled
	f.user.DailyTime = parsed
	f.user.DailySetup = "done"
	return nil
}

func roleTestContext(scenario *telegramScenario, text string) tele.Context {
	payload := ""
	if strings.HasPrefix(text, "/") {
		_, payload, _ = strings.Cut(text, " ")
	}
	return scenario.bot.NewContext(tele.Update{Message: &tele.Message{Text: text, Payload: payload, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
}

func TestTeacherRegistrationOffersDailyAndRestoresPendingTime(t *testing.T) {
	scenario := newTelegramScenario(t)
	profile := &roleProfileFake{navigationUserService: &navigationUserService{user: domain.User{ID: "42", DailySetup: "choice", DailyTime: "06:00", TeacherScheduleView: domain.ScheduleViewCompact}}}
	h := &Handler{UserService: profile, ProfileService: profile, StateManager: state.NewManager(),
		GroupService: &navigationGroupService{}, ScheduleService: &teacherScheduleScenarioService{teachers: []string{"Иванов И.И.", "Иванов А.А."}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "Вуз", Timezone: "Europe/Moscow", IsActive: true}}}}
	if err := h.HandleRole(roleTestContext(scenario, "/role")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleSelectRole, scenario.button(t, "Преподаватель"), false)
	scenario.callback(t, h.HandleUniversitySelect, scenario.button(t, "Вуз"), false)
	if err := h.HandleTextInput(roleTestContext(scenario, "Иванов")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleTeacherSelect, scenario.button(t, "Иванов И.И."), false)
	confirmation := scenario.button(t, "Подтвердить ФИО")
	scenario.callback(t, h.HandleConfirmTeacher, confirmation, false)
	if profile.saves != 1 || profile.user.TeacherID == "" || !strings.Contains(scenario.text, "Хотите получать") {
		t.Fatalf("missing daily offer: %s", scenario.text)
	}
	scenario.callback(t, h.HandleConfirmTeacher, confirmation, false)
	if profile.saves != 1 {
		t.Fatal("stale confirmation saved twice")
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Да, настроить время"), false)
	if profile.user.DailyEnabled || profile.user.DailySetup != "time" {
		t.Fatal("daily enabled before time saved")
	}
	h.StateManager = state.NewManager()
	if err := h.HandleStart(roleTestContext(scenario, "/start")); err != nil {
		t.Fatal(err)
	}
	if h.StateManager.Get(42).Step != "daily_time" {
		t.Fatal("time step not restored")
	}
	if err := h.HandleTextInput(roleTestContext(scenario, "25:00")); err != nil {
		t.Fatal(err)
	}
	if profile.user.DailyEnabled {
		t.Fatal("invalid time enabled delivery")
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Другое время"), false)
	if err := h.HandleTextInput(roleTestContext(scenario, "06:30")); err != nil {
		t.Fatal(err)
	}
	if !profile.user.DailyEnabled || profile.user.DailyTime != "06:30" || profile.user.DailySetup != "done" {
		t.Fatalf("settings %#v", profile.user)
	}
	h.StateManager = state.NewManager()
	if err := h.HandleStart(roleTestContext(scenario, "/start")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(scenario.text, "Хотите получать") || h.StateManager.Get(42).Step != "done" {
		t.Fatal("completed registration prompted again")
	}
	if err := h.beginGroupChange(roleTestContext(scenario, ""), "subscriptions", 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(scenario.text, "Моё расписание") {
		t.Fatal("teacher reached subscriptions")
	}
}

func TestDailyDeclineAndSettingsReuseSameValidation(t *testing.T) {
	scenario := newTelegramScenario(t)
	profile := &roleProfileFake{navigationUserService: &navigationUserService{user: domain.User{ID: "42", Role: domain.RoleStudent, DefaultGroupID: "g", DailySetup: "choice", DailyTime: "06:00"}}}
	h := &Handler{UserService: profile, ProfileService: profile, StateManager: state.NewManager(),
		GroupService:    &navigationGroupService{group: domain.Group{ID: "g", Name: "Group", UniversityID: "u", IsActive: true}},
		ScheduleService: &scheduleScenarioService{empty: true}, SubscriptionService: &subscriptionScenarioService{},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "Вуз", Timezone: "Europe/Moscow", IsActive: true}}}}
	if err := h.finishProfileSetup(roleTestContext(scenario, "")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Нет, позже"), false)
	if profile.user.DailyEnabled || profile.user.DailySetup != "done" {
		t.Fatal("decline not persisted")
	}
	if err := h.HandleDaily(roleTestContext(scenario, "/daily")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Настроить время"), false)
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "07:00"), false)
	if !profile.user.DailyEnabled || profile.user.DailyTime != "07:00" {
		t.Fatal(fmt.Sprintf("settings not applied %#v", profile.user))
	}
}

type onboardingSubscriptions struct {
	subscriptionScenarioService
	profile *roleProfileFake
}

func (s *onboardingSubscriptions) SubscribeAndSetDefault(_ context.Context, _ string, group string) error {
	s.profile.user.DefaultGroupID = group
	s.profile.user.Role = domain.RoleStudent
	return nil
}

func TestStudentRegistrationAsksDailyAfterGroupConfirmation(t *testing.T) {
	scenario := newTelegramScenario(t)
	profile := &roleProfileFake{navigationUserService: &navigationUserService{user: domain.User{ID: "42", Role: domain.RoleStudent, DailySetup: "choice", DailyTime: "06:00"}}}
	groups := &inputScenarioGroups{groups: []domain.Group{{ID: "g", Name: "4/147", UniversityID: "u", IsActive: true}}}
	h := &Handler{UserService: profile, ProfileService: profile, StateManager: state.NewManager(), GroupService: groups, ScheduleService: &scheduleScenarioService{empty: true}, SubscriptionService: &onboardingSubscriptions{profile: profile}, UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "Вуз", Timezone: "Europe/Moscow", IsActive: true}}}}
	if err := h.HandleStart(roleTestContext(scenario, "/start")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleSelectRole, scenario.button(t, "Студент"), false)
	scenario.callback(t, h.HandleUniversitySelect, scenario.button(t, "Вуз"), false)
	if err := h.HandleTextInput(roleTestContext(scenario, "4/147")); err != nil {
		t.Fatal(err)
	}
	if profile.user.DefaultGroupID != "" {
		t.Fatal("group saved before confirmation")
	}
	scenario.callback(t, h.HandleConfirmPrimaryGroup, scenario.button(t, "Подтвердить основную группу"), false)
	if profile.user.DefaultGroupID != "g" || !strings.Contains(scenario.text, "Хотите получать") {
		t.Fatalf("profile %#v message %s", profile.user, scenario.text)
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Да, настроить время"), false)
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Назад"), false)
	if profile.user.DailyEnabled || profile.user.DailySetup != "choice" {
		t.Fatal("back changed daily choice")
	}
	scenario.callback(t, h.HandleDailyAction, scenario.button(t, "Нет, позже"), false)
	if err := h.HandleDaily(roleTestContext(scenario, "/daily 08:00")); err != nil {
		t.Fatal(err)
	}
	if !profile.user.DailyEnabled || profile.user.DailyTime != "08:00" {
		t.Fatal("command did not save time")
	}
	if err := h.HandleDaily(roleTestContext(scenario, "/daily off")); err != nil {
		t.Fatal(err)
	}
	if profile.user.DailyEnabled {
		t.Fatal("command did not disable delivery")
	}
}

func TestTeacherBindingCancellationAndRoleRestoration(t *testing.T) {
	scenario := newTelegramScenario(t)
	profile := &roleProfileFake{navigationUserService: &navigationUserService{user: domain.User{ID: "42", Role: domain.RoleTeacher, TeacherID: "teacher", DefaultGroupID: "g", DailySetup: "done", DailyEnabled: true, DailyTime: "06:30", TeacherScheduleView: domain.ScheduleViewCompact}}, teacher: &domain.Teacher{ID: "teacher", UniversityID: "u", Name: "Иванов И.И.", NameKey: searchtext.TokenKey("Иванов И.И.")}}
	h := &Handler{UserService: profile, ProfileService: profile, StateManager: state.NewManager(), GroupService: &navigationGroupService{group: domain.Group{ID: "g", Name: "Group", UniversityID: "u", IsActive: true}}, SubscriptionService: &subscriptionScenarioService{}, ScheduleService: &roleScheduleScenarioService{teacherScheduleScenarioService: &teacherScheduleScenarioService{teachers: []string{"Иванов И.И.", "Петров П.П."}}}, UniversityService: &navigationUniversityService{universities: map[string]domain.University{"u": {ID: "u", Name: "Вуз", Timezone: "Europe/Moscow", IsActive: true}}}}
	if err := h.HandleChangeTeacher(roleTestContext(scenario, "")); err != nil {
		t.Fatal(err)
	}
	if err := h.HandleTextInput(roleTestContext(scenario, "Петров")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleCancelProfileBinding, scenario.button(t, "Отмена"), false)
	if profile.teacher.Name != "Иванов И.И." || profile.saves != 0 {
		t.Fatal("cancel replaced teacher")
	}
	if err := h.HandleRole(roleTestContext(scenario, "/role")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleSelectRole, scenario.button(t, "Студент"), false)
	if profile.user.Role != domain.RoleStudent || profile.user.TeacherID != "teacher" || !profile.user.DailyEnabled || profile.user.DailyTime != "06:30" || strings.Contains(scenario.text, "Хотите получать") {
		t.Fatalf("role switch lost settings %#v", profile.user)
	}
	if err := h.HandleRole(roleTestContext(scenario, "/role")); err != nil {
		t.Fatal(err)
	}
	scenario.callback(t, h.HandleSelectRole, scenario.button(t, "Преподаватель"), false)
	if profile.user.Role != domain.RoleTeacher || profile.saves != 0 || profile.user.DefaultGroupID != "g" {
		t.Fatal("teacher profile not restored")
	}
}

type roleScheduleScenarioService struct {
	*teacherScheduleScenarioService
}

func (s *roleScheduleScenarioService) GetScheduleForGroupRange(context.Context, string, time.Time, time.Time) (map[time.Time][]domain.Lesson, error) {
	return map[time.Time][]domain.Lesson{}, nil
}
