//go:build integration

package handlers

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	tele "gopkg.in/telebot.v3"
)

type runtimeJourney struct {
	h                    *Handler
	telegram             *telegramScenario
	db, botDB, privacyDB *sqlx.DB
	ctx                  context.Context
	date                 time.Time
}

func newRuntimeJourney(t *testing.T) *runtimeJourney {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	base, err := sqlx.ConnectContext(ctx, "pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { base.Close() })
	name := "bot_journey_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := base.ExecContext(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	roles := []string{name + "_bot", name + "_admin", name + "_parser", name + "_privacy"}
	var createdRoles []string
	t.Cleanup(func() {
		if _, err := base.Exec(`DROP DATABASE ` + pgx.Identifier{name}.Sanitize() + ` WITH (FORCE)`); err != nil {
			t.Error(err)
		}
		for _, role := range createdRoles {
			if _, err := base.Exec(`DROP ROLE ` + pgx.Identifier{role}.Sanitize()); err != nil {
				t.Error(err)
			}
		}
	})
	password := uuid.NewString()
	for _, role := range roles {
		if _, err := base.ExecContext(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN PASSWORD '`+password+`'`); err != nil {
			t.Fatal(err)
		}
		createdRoles = append(createdRoles, role)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	query := parsed.Query()
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	db, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyRuntimeGrants(ctx, db, roles[0], roles[1], roles[2], roles[3]); err != nil {
		t.Fatal(err)
	}
	connectRole := func(role string) *sqlx.DB {
		parsed.User = url.UserPassword(role, password)
		connection, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { connection.Close() })
		return connection
	}
	botDB, privacyDB := connectRole(roles[0]), connectRole(roles[3])
	schedule := service.NewScheduleService(repository.NewLessonRepository(botDB), repository.NewSemesterRepository(botDB), repository.NewGroupRepository(botDB))
	h := NewHandler(schedule, service.NewUserService(repository.NewUserRepository(botDB)), service.NewGroupService(repository.NewGroupRepository(botDB)), service.NewUniversityService(repository.NewUniversityRepository(botDB)), state.NewManager(), service.NewSubscriptionService(repository.NewSubscriptionRepository(botDB)), service.NewSupportRequestService(repository.NewSupportRequestRepository(botDB)), service.NewMetricsService(repository.NewMetricsRepository(botDB)), service.NewChatProfileService(repository.NewChatProfileRepository(botDB)), nil, "", "https://example.test/project")
	location, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO groups(id,university_id,name) VALUES('journey-a','isuct','4/147'),('journey-b','isuct','4/245');
		INSERT INTO users(id,username,is_admin,admin_role) VALUES('43','Synthetic owner',TRUE,'owner');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO semesters(id,university_id,external_id,name,start_date,end_date) VALUES('journey-term','isuct','journey-term','Test term',$1,$2)`, date.AddDate(0, 0, -60), date.AddDate(0, 0, 60)); err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 7; day++ {
		for subgroup := 0; subgroup <= 2; subgroup++ {
			if _, err := db.ExecContext(ctx, `INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,teacher,room,type,subgroup) VALUES($1,'isuct','journey-term','journey-a',$2,$3,$4,'every',$5,'Иванов И.И.','А101','lecture',$6)`, fmt.Sprintf("journey-%d-%d", day, subgroup), day, []string{"08:00", "12:10", "14:00"}[subgroup], []string{"09:35", "13:45", "15:35"}[subgroup], fmt.Sprintf("Предмет подгруппы %d", subgroup), subgroup); err != nil {
				t.Fatal(err)
			}
		}
	}
	return &runtimeJourney{h: h, telegram: newTelegramScenario(t), db: db, botDB: botDB, privacyDB: privacyDB, ctx: ctx, date: date}
}

func (j *runtimeJourney) message(t *testing.T, handler tele.HandlerFunc, command, payload string) {
	t.Helper()
	c := j.telegram.bot.NewContext(tele.Update{Message: &tele.Message{ID: 1, Text: command, Payload: payload, Sender: &tele.User{ID: 42, Username: "Synthetic student"}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
	if err := handler(c); err != nil {
		t.Fatal(err)
	}
	j.requireNoError(t)
}

func (j *runtimeJourney) callback(t *testing.T, handler tele.HandlerFunc, args string) {
	t.Helper()
	j.telegram.callback(t, handler, args, false)
	j.requireNoError(t)
}

func (j *runtimeJourney) requireNoError(t *testing.T) {
	t.Helper()
	j.telegram.mu.Lock()
	defer j.telegram.mu.Unlock()
	for _, text := range []string{"Не удалось", "Ошибка сервера"} {
		if strings.Contains(j.telegram.text, text) {
			t.Fatalf("bot returned an error: %s", j.telegram.text)
		}
	}
}

func (j *runtimeJourney) onboard(t *testing.T) {
	t.Helper()
	j.message(t, j.h.HandleStart, "/start", "")
	current := j.h.StateManager.Get(42)
	j.callback(t, j.h.HandleUniversitySelect, "isuct|"+current.FlowNonce)
	j.message(t, j.h.HandleTextInput, "4/147", "")
	current = j.h.StateManager.Get(42)
	if current.Step != "confirming_primary_group" {
		t.Fatalf("missing confirmation: %+v", current)
	}
	j.callback(t, j.h.HandleConfirmPrimaryGroup, "save|"+current.FlowNonce)
	user, err := j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || user == nil || user.DefaultGroupID != "journey-a" || user.ReminderEnabled {
		t.Fatalf("onboarding result: %+v %v", user, err)
	}
}
