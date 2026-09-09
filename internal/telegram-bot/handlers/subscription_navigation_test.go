package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type subscriptionScenarioService struct {
	*service.SubscriptionService
	items        []domain.GroupSubscription
	subscribed   []string
	defaultGroup string
}

func (s *subscriptionScenarioService) GetGroupSubscriptions(context.Context, string) ([]domain.GroupSubscription, error) {
	return s.items, nil
}

func (s *subscriptionScenarioService) Subscribe(_ context.Context, _ string, groupID string, _ string) error {
	s.subscribed = append(s.subscribed, groupID)
	return nil
}

func (s *subscriptionScenarioService) SubscribeAndSetDefault(_ context.Context, _ string, groupID string) error {
	s.defaultGroup = groupID
	return nil
}

type scheduleScenarioQuery struct {
	group    string
	from, to time.Time
}

type scheduleScenarioService struct {
	*service.ScheduleService
	queries []scheduleScenarioQuery
	empty   bool
}

func (s *scheduleScenarioService) GetScheduleForGroupRange(_ context.Context, groupID string, from, to time.Time) (map[time.Time][]domain.Lesson, error) {
	s.queries = append(s.queries, scheduleScenarioQuery{groupID, from, to})
	if s.empty {
		return map[time.Time][]domain.Lesson{}, nil
	}
	return map[time.Time][]domain.Lesson{dateAtLocation(from, from.Location()): {
		{Subject: "Общее занятие", TimeStart: "08:00", TimeEnd: "09:35", Type: domain.LessonTypeLecture},
		{Subject: "Моя подгруппа", TimeStart: "09:50", TimeEnd: "11:25", Type: domain.LessonTypePractice, Subgroup: 2},
		{Subject: "Другая подгруппа", TimeStart: "09:50", TimeEnd: "11:25", Type: domain.LessonTypePractice, Subgroup: 1},
	}}, nil
}

type telegramScenario struct {
	bot      *tele.Bot
	mu       sync.Mutex
	markup   tele.ReplyMarkup
	files    map[string]string
	methods  []string
	text     string
	payloads []map[string]string
	chatRole string
}

func newTelegramScenario(t *testing.T) *telegramScenario {
	t.Helper()
	scenario := &telegramScenario{files: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := map[string]string{}
		files := map[string]string{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Errorf("parse Telegram upload: %v", err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			for key := range r.MultipartForm.Value {
				body[key] = r.FormValue(key)
			}
			for _, headers := range r.MultipartForm.File {
				for _, header := range headers {
					file, err := header.Open()
					if err != nil {
						t.Errorf("open upload: %v", err)
						continue
					}
					data, err := io.ReadAll(file)
					_ = file.Close()
					if err != nil {
						t.Errorf("read upload: %v", err)
					}
					files[header.Filename] = string(data)
				}
			}
		} else {
			var raw map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Errorf("decode Telegram request: %v", err)
			}
			for key, value := range raw {
				var text string
				if err := json.Unmarshal(value, &text); err != nil {
					text = string(value)
				}
				body[key] = text
			}
		}
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		scenario.mu.Lock()
		scenario.methods = append(scenario.methods, method)
		scenario.payloads = append(scenario.payloads, body)
		for name, data := range files {
			scenario.files[name] = data
		}
		if raw, ok := body["reply_markup"]; ok {
			if err := json.Unmarshal([]byte(raw), &scenario.markup); err != nil {
				t.Errorf("decode markup: %v", err)
			}
		}
		if body["text"] != "" {
			scenario.text = body["text"]
		}
		scenario.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if method == "getChatMember" {
			role := scenario.chatRole
			if role == "" {
				role = "member"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"status": role, "user": map[string]any{"id": 42}}})
			return
		}
		if method == "answerCallbackQuery" || method == "deleteMessage" || method == "answerInlineQuery" || method == "setChatMenuButton" || method == "setMyCommands" {
			_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
			return
		}
		if method == "sendPhoto" {
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":7,"chat":{"id":42,"type":"private"},"photo":[{"file_id":"photo","file_unique_id":"photo","width":1900,"height":800}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":7,"chat":{"id":42,"type":"private"},"text":"schedule"}}`)
	}))
	t.Cleanup(server.Close)
	var err error
	scenario.bot, err = tele.NewBot(tele.Settings{URL: server.URL, Token: "test-token", Client: server.Client(), Offline: true, Synchronous: true})
	if err != nil {
		t.Fatal(err)
	}
	return scenario
}

func (s *telegramScenario) callback(t *testing.T, handler tele.HandlerFunc, args string, visual bool) {
	t.Helper()
	message := &tele.Message{ID: 7, Text: "schedule", Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}
	if visual {
		message.Text = ""
		message.Photo = &tele.Photo{File: tele.FromURL("https://example.test/schedule.png")}
	}
	ctx := s.bot.NewContext(tele.Update{Callback: &tele.Callback{
		ID: "callback", Data: args, Sender: &tele.User{ID: 42}, Message: message,
	}})
	if err := handler(ctx); err != nil {
		t.Fatalf("callback %q: %v", args, err)
	}
}

func (s *telegramScenario) button(t *testing.T, label string) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, row := range s.markup.InlineKeyboard {
		for _, button := range row {
			if button.Text == label || strings.TrimPrefix(button.Text, "•") == label {
				_, args, _ := strings.Cut(button.Data, "|")
				return args
			}
		}
	}
	t.Fatalf("button %q not found: %#v", label, s.markup.InlineKeyboard)
	return ""
}

func (s *telegramScenario) requireActions(t *testing.T, actions ...string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, row := range s.markup.InlineKeyboard {
		for _, button := range row {
			endpoint, _, _ := strings.Cut(button.Data, "|")
			seen[strings.TrimPrefix(endpoint, "\f")] = true
		}
	}
	for _, action := range actions {
		if !seen[action] {
			t.Errorf("missing action %q in %#v", action, seen)
		}
	}
}

func TestSecondarySubscriptionKeepsGroupAcrossScheduleNavigation(t *testing.T) {
	for _, format := range []domain.ScheduleViewFormat{domain.ScheduleViewCompact, domain.ScheduleViewVisual} {
		t.Run(string(format), func(t *testing.T) {
			for _, empty := range []bool{false, true} {
				t.Run(fmt.Sprint("empty=", empty), func(t *testing.T) {
					item := domain.GroupSubscription{GroupID: "secondary", GroupName: "4/147", UniversityID: "isuct", UniversityName: "ИГХТУ", IsActive: true, ScheduleViewFormat: format, Subgroup: 2}
					subscriptions := &subscriptionScenarioService{items: []domain.GroupSubscription{
						{GroupID: "primary", GroupName: "4/147", UniversityID: "another", IsDefault: true, IsActive: true}, item,
					}}
					schedules := &scheduleScenarioService{empty: empty}
					h := &Handler{SubscriptionService: subscriptions, ScheduleService: schedules, StateManager: state.NewManager(), UniversityService: &navigationUniversityService{universities: map[string]domain.University{
						"isuct": {ID: "isuct", Name: "ИГХТУ", Timezone: "Europe/Moscow", IsActive: true},
					}}}
					h.StateManager.Set(42, &dto.UserState{GroupID: "primary", UniversityID: "another", Step: "done"})
					s := newTelegramScenario(t)
					visual := format == domain.ScheduleViewVisual
					token := keyboards.GroupToken(item.GroupID)
					s.callback(t, h.HandleSubscriptionSchedule, token+"|today|0", visual)
					s.requireActions(t, "schedule_date", "schedule_week", "open_calendar", "open_schedule_exports", "open_main_menu")
					s.callback(t, h.HandleScheduleDateSelect, s.button(t, "→"), visual)
					s.callback(t, h.HandleScheduleWeekSelect, s.button(t, "Неделя"), visual)
					s.callback(t, h.HandleScheduleWeekSelect, s.button(t, "Две недели"), visual)
					last := schedules.queries[len(schedules.queries)-1]
					if last.to.Sub(last.from) != 13*24*time.Hour {
						t.Fatalf("two-week range = %s .. %s", last.from, last.to)
					}
					s.callback(t, h.HandleOpenWeekday, s.button(t, "Выбрать день"), visual)
					s.mu.Lock()
					dayData := strings.SplitN(s.markup.InlineKeyboard[4][0].Data, "|", 2)[1]
					s.mu.Unlock()
					s.callback(t, h.HandleSchedulePeriodDateSelect, dayData, visual)
					s.requireActions(t, "schedule_date", "schedule_week", "open_calendar", "open_schedule_exports")
					s.button(t, "Назад к двум неделям")
					s.callback(t, h.HandleOpenCalendar, s.button(t, "Выбрать дату"), visual)
					s.callback(t, h.HandleCalendarMonth, s.button(t, "›"), visual)
					s.callback(t, h.HandleScheduleDateSelect, s.button(t, "1"), visual)
					s.requireActions(t, "schedule_date", "schedule_week", "open_calendar", "open_schedule_exports")
					s.callback(t, h.HandleOpenScheduleExports, s.button(t, "Скачать расписание"), visual)
					s.callback(t, h.HandleDownloadSchedule, s.button(t, "Календарь ICS"), visual)
					s.mu.Lock()
					found := false
					for name, data := range s.files {
						if !strings.HasSuffix(name, ".ics") {
							continue
						}
						found = true
						if !strings.Contains(data, "BEGIN:VCALENDAR") || strings.Contains(data, "Другая подгруппа") {
							t.Errorf("invalid calendar or subgroup filter: %s", data)
						}
						if !empty && !strings.Contains(data, "T050000Z") {
							t.Error("calendar must convert 08:00 Moscow time to 05:00 UTC")
						}
					}
					s.mu.Unlock()
					if !found {
						t.Fatal("calendar was not uploaded")
					}
					s.callback(t, h.HandleOpenScheduleExports, s.button(t, "Назад к форматам"), visual)
					s.callback(t, h.HandleBackToSchedule, s.button(t, "Назад к расписанию"), visual)
					s.requireActions(t, "schedule_date", "schedule_week", "open_calendar", "open_schedule_exports")
					for _, query := range schedules.queries {
						if query.group != item.GroupID {
							t.Errorf("navigation switched to %q", query.group)
						}
					}
					if current := h.StateManager.Get(42); current.GroupID != "primary" || subscriptions.defaultGroup != "" {
						t.Fatal("viewing another group changed the primary profile")
					}
					args := s.button(t, "→")
					subscriptions.items = subscriptions.items[:1]
					count := len(schedules.queries)
					s.callback(t, h.HandleScheduleDateSelect, args, visual)
					if len(schedules.queries) != count {
						t.Fatal("removed subscription fell back to another group")
					}
				})
			}
		})
	}
}

func TestCalendarExportLegacyButtonAndInactiveGroup(t *testing.T) {
	subscriptions := &subscriptionScenarioService{items: []domain.GroupSubscription{{GroupID: "group", IsActive: true, UniversityID: "isuct"}}}
	h := &Handler{SubscriptionService: subscriptions, ScheduleService: &scheduleScenarioService{empty: true}, UniversityService: &navigationUniversityService{universities: map[string]domain.University{
		"isuct": {ID: "isuct", IsActive: true},
	}}}
	s := newTelegramScenario(t)
	s.callback(t, h.HandleDownloadScheduleICS, keyboards.GroupToken("group")+"|2026-09-01|1", false)
	s.mu.Lock()
	if !slices.Contains(s.methods, "sendDocument") {
		t.Error("old calendar download button no longer works")
	}
	s.mu.Unlock()
	subscriptions.items[0].IsActive = false
	if target, err := h.downloadTarget(context.Background(), s.bot.NewContext(tele.Update{Message: &tele.Message{Sender: &tele.User{ID: 42}}}), keyboards.GroupToken("group")); err == nil || target != nil {
		t.Error("inactive group must not be exported")
	}
}

func TestGroupSearchUsesFullNavigationWithoutSubscribing(t *testing.T) {
	subscriptions := &subscriptionScenarioService{}
	schedules := &scheduleScenarioService{empty: true}
	groups := &inputScenarioGroups{groups: []domain.Group{
		{ID: "primary", Name: "3/42", UniversityID: "isuct", IsActive: true},
		{ID: "found", Name: "4/147", UniversityID: "isuct", IsActive: true},
	}}
	h := &Handler{
		StateManager: state.NewManager(), SubscriptionService: subscriptions, ScheduleService: schedules, GroupService: groups,
		UserService:       &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "primary"}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}}},
	}
	current := &dto.UserState{GroupID: "primary", UniversityID: "isuct", Step: "awaiting_search_query", SearchType: dto.SearchTypeGroup, SearchQuery: "ИГХТУ 4 курс 147 группа"}
	h.StateManager.Set(42, current)
	s := newTelegramScenario(t)
	ctx := s.bot.NewContext(tele.Update{Message: &tele.Message{Text: current.SearchQuery, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
	if err := h.HandleSearchResult(ctx, current); err != nil {
		t.Fatal(err)
	}
	s.requireActions(t, "schedule_week", "open_calendar", "open_weekday", "open_schedule_exports")
	s.callback(t, h.HandleScheduleWeekSelect, s.button(t, "Две недели"), true)
	s.callback(t, h.HandleOpenCalendar, s.button(t, "Выбрать дату"), true)
	s.callback(t, h.HandleCalendarMonth, s.button(t, "›"), true)
	s.callback(t, h.HandleSchedulePeriodDateSelect, s.button(t, "1"), true)
	s.requireActions(t, "schedule_date", "schedule_week", "open_calendar", "open_schedule_exports")
	s.callback(t, h.HandleOpenScheduleExports, s.button(t, "Скачать расписание"), true)
	if args := s.button(t, "Календарь ICS"); !strings.HasPrefix(args, "ics|p"+keyboards.GroupToken("found")+"|") {
		t.Fatalf("search export lost public group reference: %q", args)
	}
	s.callback(t, h.HandleDownloadSchedule, s.button(t, "Календарь ICS"), true)
	s.callback(t, h.HandleOpenScheduleExports, s.button(t, "Назад к форматам"), true)
	s.callback(t, h.HandleBackToSchedule, s.button(t, "Назад к расписанию"), true)
	if subscriptions.defaultGroup != "" || len(subscriptions.subscribed) != 0 || h.StateManager.Get(42).GroupID != "primary" {
		t.Fatal("search navigation modified subscriptions or primary group")
	}
	for _, query := range schedules.queries {
		if query.group != "found" {
			t.Errorf("search navigated to %s", query.group)
		}
	}
	groups.groups[1].IsActive = false
	count := len(schedules.queries)
	s.callback(t, h.HandleScheduleDateSelect, s.button(t, "→"), true)
	if len(schedules.queries) != count {
		t.Fatal("disabled public group is still visible")
	}
}
