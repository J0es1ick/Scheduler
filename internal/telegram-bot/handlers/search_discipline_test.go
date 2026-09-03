package handlers

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type disciplineScheduleService struct {
	*service.ScheduleService
	days map[time.Time][]domain.Lesson
}

func (s *disciplineScheduleService) GetScheduleForGroupRange(
	context.Context,
	string,
	time.Time,
	time.Time,
) (map[time.Time][]domain.Lesson, error) {
	return s.days, nil
}

func TestDisciplineSearchUsesPrimarySubgroupAndSortsDays(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	first := time.Date(2026, time.September, 1, 0, 0, 0, 0, location)
	second := first.AddDate(0, 0, 2)
	schedules := &disciplineScheduleService{days: map[time.Time][]domain.Lesson{
		second: {
			{Subject: "Математика", Subgroup: 2, TimeStart: "09:50", TimeEnd: "11:25"},
			{Subject: "Математика", Subgroup: 1, TimeStart: "12:10", TimeEnd: "13:45"},
		},
		first: {
			{Subject: "Высшая математика", Subgroup: 0, TimeStart: "08:00", TimeEnd: "09:35"},
		},
	}}
	subscriptions := &subscriptionScenarioService{items: []domain.GroupSubscription{
		{GroupID: "primary", IsDefault: true, IsActive: true, Subgroup: 2},
	}}
	h := &Handler{
		StateManager:        state.NewManager(),
		ScheduleService:     schedules,
		SubscriptionService: subscriptions,
		UserService: &navigationUserService{user: domain.User{
			ID: "42", DefaultGroupID: "primary",
		}},
		GroupService: &navigationGroupService{group: domain.Group{
			ID: "primary", UniversityID: "university", Name: "TEST", IsActive: true,
		}},
		UniversityService: &navigationUniversityService{universities: map[string]domain.University{
			"university": {ID: "university", IsActive: true, Timezone: "Europe/Moscow"},
		}},
	}
	current := &dto.UserState{
		GroupID: "primary", UniversityID: "university", Step: "awaiting_search_query",
		SearchType: dto.SearchTypeDiscipline, SearchQuery: "матиматика", FlowNonce: "search",
	}
	h.StateManager.Set(42, current)
	scenario := newTelegramScenario(t)
	ctx := scenario.bot.NewContext(tele.Update{Message: &tele.Message{
		Text: current.SearchQuery, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
	}})
	if err := h.HandleSearchResult(ctx, current); err != nil {
		t.Fatal(err)
	}
	scenario.mu.Lock()
	text := scenario.text
	scenario.mu.Unlock()
	if strings.Contains(text, "12:10") {
		t.Fatalf("discipline search included another subgroup: %s", text)
	}
	firstIndex := strings.Index(text, "01.09.2026")
	secondIndex := strings.Index(text, "03.09.2026")
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("discipline days are not sorted: %s", text)
	}
	if current := h.StateManager.Get(42); current == nil || current.Step != "done" {
		t.Fatalf("search state was not completed: %+v", current)
	}
}

func TestSearchAvailabilityIsRefreshedAtEntryAndExecution(t *testing.T) {
	for _, execution := range []bool{false, true} {
		t.Run(fmt.Sprintf("execution=%t", execution), func(t *testing.T) {
			scenario := newTelegramScenario(t)
			h := &Handler{
				StateManager: state.NewManager(),
				ScheduleService: &disciplineScheduleService{days: map[time.Time][]domain.Lesson{
					time.Now(): {{Subject: "Математика"}},
				}},
				UserService: &navigationUserService{user: domain.User{ID: "42", DefaultGroupID: "primary"}},
				GroupService: &navigationGroupService{group: domain.Group{
					ID: "primary", UniversityID: "university", Name: "TEST", IsActive: false,
				}},
				UniversityService: &navigationUniversityService{universities: map[string]domain.University{
					"university": {ID: "university", Name: "University", IsActive: true},
				}},
			}
			contextValue := scenario.bot.NewContext(tele.Update{Message: &tele.Message{
				Text: "математика", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate},
			}})
			if execution {
				current := &dto.UserState{
					GroupID: "primary", UniversityID: "university", GroupActive: true,
					Step: "awaiting_search_query", SearchType: dto.SearchTypeDiscipline, SearchQuery: "математика",
				}
				h.StateManager.Set(42, current)
				if err := h.HandleSearchResult(contextValue, current); err != nil {
					t.Fatal(err)
				}
			} else if err := h.HandleSearch(contextValue); err != nil {
				t.Fatal(err)
			}
			stored := h.StateManager.Get(42)
			if stored == nil || stored.Step != "done" || stored.GroupActive {
				t.Fatalf("inactive search context was not reset: %+v", stored)
			}
			scenario.mu.Lock()
			text := scenario.text
			scenario.mu.Unlock()
			if !strings.Contains(text, "недоступ") {
				t.Fatalf("inactive search message = %q", text)
			}
		})
	}
}
