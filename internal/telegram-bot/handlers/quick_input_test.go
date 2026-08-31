package handlers

import (
	"slices"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestParseQuickInput(t *testing.T) {
	tests := []struct {
		input string
		kind  quickInputKind
		value string
	}{
		{input: "08.01.2002", kind: quickInputDate, value: "08.01.2002"},
		{input: "01.10.02", kind: quickInputDate, value: "01.10.02"},
		{input: "Понедельник", kind: quickInputWeekday, value: "1"},
		{input: "пн", kind: quickInputWeekday, value: "1"},
		{input: "-7", kind: quickInputOffset, value: "-7"},
		{input: "-2", kind: quickInputOffset, value: "-2"},
		{input: "0", kind: quickInputOffset, value: "0"},
		{input: "2", kind: quickInputOffset, value: "2"},
		{input: "7", kind: quickInputOffset, value: "7"},
		{input: "4-185", kind: quickInputGroup, value: "4-185"},
		{input: "4/185", kind: quickInputGroup, value: "4/185"},
		{input: `4\185`, kind: quickInputGroup, value: `4\185`},
		{input: "4 185", kind: quickInputGroup, value: "4 185"},
		{input: "4–185", kind: quickInputGroup, value: "4–185"},
		{input: "4—185", kind: quickInputGroup, value: "4—185"},
		{input: "4 курс 185 группа", kind: quickInputGroup, value: "4 курс 185 группа"},
		{input: "ИГЭУ 1-ЭЭ-В", kind: quickInputGroup, value: "ИГЭУ 1-ЭЭ-В"},
		{input: "Константинов Е.С.", kind: quickInputTeacher, value: "Константинов Е.С."},
		{input: "Константинов", kind: quickInputTeacher, value: "Константинов"},
		{input: "Поиск Конст", kind: quickInputTeacher, value: "Конст"},
		{input: "Сегодня", kind: quickInputOffset, value: "0"},
		{input: "Завтра", kind: quickInputOffset, value: "1"},
		{input: "Неделя", kind: quickInputPeriod, value: "7"},
		{input: "Две недели", kind: quickInputPeriod, value: "14"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := parseQuickInput(test.input)
			if result.kind != test.kind || result.value != test.value {
				t.Fatalf("parseQuickInput(%q) = %#v, want kind=%q value=%q", test.input, result, test.kind, test.value)
			}
		})
	}
}

func TestParseQuickInputRejectsAmbiguousText(t *testing.T) {
	for _, input := range []string{"-8", "8", "таймер 04:19", "Поиск", "обычный текст с лишними словами", ""} {
		if result := parseQuickInput(input); result.kind != quickInputNone {
			t.Fatalf("parseQuickInput(%q) = %#v, want no command", input, result)
		}
	}
}

func TestQuickGroupSeparatorsResolveToSameName(t *testing.T) {
	for _, input := range []string{"4-185", "4/185", `4\185`, "4 185", "4–185", "4—185", "4 курс 185 группа"} {
		variants := groupQueryVariants(input)
		if !slices.Contains(variants, "4/185") || !slices.Contains(variants, "4-185") {
			t.Fatalf("groupQueryVariants(%q) = %#v, want slash and dash variants", input, variants)
		}
	}
}

func TestQuickGroupUsesRestoredPrimaryUniversity(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		groups       []domain.Group
		universities map[string]domain.University
		wantGroup    string
	}{
		{
			name:  "same university",
			input: "NEW-1",
			groups: []domain.Group{
				{ID: "old", Name: "OLD-1", UniversityID: "isuct", IsActive: true},
				{ID: "new", Name: "NEW-1", UniversityID: "isuct", IsActive: true},
			},
			universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}},
			wantGroup:    "new",
		},
		{
			name:  "other university",
			input: "NEW-1",
			groups: []domain.Group{
				{ID: "old", Name: "OLD-1", UniversityID: "isuct", IsActive: true},
				{ID: "new", Name: "NEW-1", UniversityID: "ispu", IsActive: true},
			},
			universities: map[string]domain.University{
				"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
				"ispu":  {ID: "ispu", Name: "ИГЭУ", IsActive: true},
			},
		},
		{
			name:  "separator normalization",
			input: "4 185",
			groups: []domain.Group{
				{ID: "old", Name: "OLD-1", UniversityID: "isuct", IsActive: true},
				{ID: "new", Name: "4-185", UniversityID: "isuct", IsActive: true},
			},
			universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}},
			wantGroup:    "new",
		},
		{
			name:  "recover from inactive group",
			input: "NEW-1",
			groups: []domain.Group{
				{ID: "old", Name: "OLD-1", UniversityID: "isuct", IsActive: false},
				{ID: "new", Name: "NEW-1", UniversityID: "isuct", IsActive: true},
			},
			universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}},
			wantGroup:    "new",
		},
		{
			name:  "qualified other university",
			input: "ИГЭУ NEW-1",
			groups: []domain.Group{
				{ID: "old", Name: "OLD-1", UniversityID: "isuct", IsActive: true},
				{ID: "new", Name: "NEW-1", UniversityID: "ispu", IsActive: true},
			},
			universities: map[string]domain.University{
				"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
				"ispu":  {ID: "ispu", Name: "ИГЭУ", IsActive: true},
			},
			wantGroup: "new",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subscriptions := &subscriptionScenarioService{}
			handler := &Handler{
				StateManager:        state.NewManager(),
				UserService:         &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "old"}},
				GroupService:        &inputScenarioGroups{groups: test.groups},
				UniversityService:   &navigationUniversityService{universities: test.universities},
				SubscriptionService: subscriptions,
			}
			scenario := newTelegramScenario(t)
			context := scenario.bot.NewContext(tele.Update{Message: &tele.Message{
				Text: test.input, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
			}})
			if err := handler.HandleTextInput(context); err != nil {
				t.Fatal(err)
			}
			if subscriptions.defaultGroup != test.wantGroup {
				t.Fatalf("selected group=%q, want %q", subscriptions.defaultGroup, test.wantGroup)
			}
		})
	}
}
