package handlers

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

func TestQualifiedGroupInput(t *testing.T) {
	universities := []domain.University{
		{ID: "isuct", Name: "ИГХТУ", FullName: "Ивановский химический университет", IsActive: true},
		{ID: "ispu", Name: "ИГЭУ", IsActive: true},
	}
	for index := 0; index < 40; index++ {
		universities = append(universities, domain.University{ID: fmt.Sprintf("uni-%d", index), Name: fmt.Sprintf("ВУЗ %d", index), IsActive: true})
	}
	for _, test := range []struct {
		input, university, group string
	}{
		{"ИГХТУ 4/147", "isuct", "4/147"},
		{"игхту 4 147", "isuct", "4/147"},
		{"  ИГХТУ  4 курс 147 группа  ", "isuct", "4/147"},
		{"isuct:4\\147", "isuct", "4/147"},
		{"Ивановский химический университет 4/147", "isuct", "4/147"},
		{"ИГЭУ 1 ЭЭ В", "ispu", "1-ээ-в"},
		{"ИГЭУ 4/147", "ispu", "4/147"},
		{"ВУЗ 30 4 курс 147 группа", "uni-30", "4/147"},
	} {
		t.Run(test.input, func(t *testing.T) {
			university, query, err := resolveGroupInput(test.input, "ispu", true, universities)
			if err != nil {
				t.Fatal(err)
			}
			if university.ID != test.university || !slices.Contains(groupQueryVariants(query), test.group) {
				t.Fatalf("resolved %s %v, want %s %s", university.ID, groupQueryVariants(query), test.university, test.group)
			}
		})
	}
	for _, input := range []string{"4/147", "НеизвестныйВуз 4/147", "ИГХТУ", "ИГХТУ2 4/147"} {
		if _, _, err := resolveGroupInput(input, "isuct", true, universities); err == nil {
			t.Errorf("ambiguous/unqualified input %q was accepted", input)
		}
	}
	if university, query, err := resolveGroupInput("4/147", "isuct", false, universities); err != nil || university.ID != "isuct" || query != "4/147" {
		t.Fatalf("initial setup must accept group without university: %v %q %v", university, query, err)
	}
	universities = append(universities, domain.University{ID: "other-isuct", Name: "ИГХТУ", IsActive: true})
	if _, _, err := resolveGroupInput("ИГХТУ 4/147", "isuct", true, universities); err == nil || !strings.Contains(err.Error(), "other-isuct") {
		t.Fatalf("duplicate abbreviation must request explicit code: %v", err)
	}
	if university, _, err := resolveGroupInput("isuct 4/147", "", true, universities); err != nil || university.ID != "isuct" {
		t.Fatalf("explicit code must resolve duplicate abbreviation: %v", err)
	}
	universities[0].IsActive = false
	if _, _, err := resolveGroupInput("isuct 4/147", "", true, universities); err == nil {
		t.Fatal("inactive university accepted")
	}
}

type inputScenarioGroups struct {
	*service.GroupService
	groups []domain.Group
}

func (s *inputScenarioGroups) GetGroupByID(_ context.Context, id string) (*domain.Group, error) {
	for _, group := range s.groups {
		if group.ID == id {
			return &group, nil
		}
	}
	return nil, nil
}

func (s *inputScenarioGroups) GetGroupByName(_ context.Context, universityID, name string) (*domain.Group, error) {
	for _, group := range s.groups {
		if group.UniversityID == universityID && strings.EqualFold(group.Name, name) {
			return &group, nil
		}
	}
	return nil, nil
}

func (s *inputScenarioGroups) FindActiveByName(context.Context, string, string) ([]domain.Group, error) {
	return nil, nil
}

func (s *inputScenarioGroups) GetActiveGroupByToken(_ context.Context, token string) (*domain.Group, error) {
	for _, group := range s.groups {
		if group.IsActive && keyboards.GroupToken(group.ID) == token {
			return &group, nil
		}
	}
	return nil, nil
}

func TestAddingQualifiedSubscriptionDoesNotChangePrimaryGroup(t *testing.T) {
	for _, input := range []string{"ИГХТУ 4/147", "ИГХТУ 4 147", "ИГХТУ 4 курс 147 группа"} {
		t.Run(input, func(t *testing.T) {
			subscriptions := &subscriptionScenarioService{}
			h := &Handler{
				StateManager: state.NewManager(), SubscriptionService: subscriptions,
				UserService: &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "old"}},
				GroupService: &inputScenarioGroups{groups: []domain.Group{
					{ID: "old", Name: "4/147", UniversityID: "ispu", IsActive: true},
					{ID: "new", Name: "4/147", UniversityID: "isuct", IsActive: true},
				}},
				UniversityService: &navigationUniversityService{universities: map[string]domain.University{
					"ispu": {ID: "ispu", Name: "ИГЭУ", IsActive: true}, "isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true},
				}},
			}
			s := newTelegramScenario(t)
			s.callback(t, h.HandleAddSubscription, "2", false)
			s.mu.Lock()
			prompt := s.text
			s.mu.Unlock()
			if !strings.Contains(prompt, "ИГХТУ 4 курс 147 группа") {
				t.Fatal("group input prompt lacks examples")
			}
			if err := h.HandleTextInput(s.bot.NewContext(tele.Update{Message: &tele.Message{Text: input, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(subscriptions.subscribed, []string{"new"}) || subscriptions.defaultGroup != "" {
				t.Fatalf("subscription resolved wrong group or changed default: %#v", subscriptions)
			}
			current := h.StateManager.Get(42)
			if current.GroupID != "old" || current.UniversityID != "ispu" || current.Step != "done" {
				t.Fatalf("primary profile not restored: %#v", current)
			}
		})
	}
}

func TestInvalidGroupInputCanBeCancelledWithoutLeavingDialogState(t *testing.T) {
	h := &Handler{
		StateManager: state.NewManager(), SubscriptionService: &subscriptionScenarioService{},
		UserService:       &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "old"}},
		GroupService:      &inputScenarioGroups{groups: []domain.Group{{ID: "old", Name: "4/147", UniversityID: "isuct", IsActive: true}}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{"isuct": {ID: "isuct", Name: "ИГХТУ", IsActive: true}}},
	}
	s := newTelegramScenario(t)
	s.callback(t, h.HandleAddSubscription, "2", false)
	if err := h.HandleTextInput(s.bot.NewContext(tele.Update{Message: &tele.Message{Text: "4/147", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})); err != nil {
		t.Fatal(err)
	}
	s.requireActions(t, "cancel_group_change")
	args := s.button(t, "Назад")
	if !strings.HasPrefix(args, "subscriptions|2|") {
		t.Fatalf("back lost subscription page: %q", args)
	}
	s.callback(t, h.HandleCancelGroupChange, args, false)
	if current := h.StateManager.Get(42); current.Step != "done" || current.GroupID != "old" {
		t.Fatalf("cancellation left wrong state: %#v", current)
	}
}
