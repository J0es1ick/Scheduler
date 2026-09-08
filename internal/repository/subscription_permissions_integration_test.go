//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func TestGroupSelectionWithRuntimeBotPermissions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := openRepositoryIntegrationDB(t, ctx)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	roles := []string{"subscription_bot_" + suffix, "subscription_admin_" + suffix, "subscription_parser_" + suffix, "subscription_privacy_" + suffix}
	password := uuid.NewString()
	for _, role := range roles {
		quoted := pgx.Identifier{role}.Sanitize()
		if _, err := db.ExecContext(ctx, `CREATE ROLE `+quoted+` LOGIN PASSWORD '`+password+`'`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec(`DROP OWNED BY ` + quoted); err != nil {
				t.Error(err)
			}
			if _, err := db.Exec(`DROP ROLE ` + quoted); err != nil {
				t.Error(err)
			}
		})
	}
	if err := database.ApplyRuntimeGrants(ctx, db, roles[0], roles[1], roles[2], roles[3]); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.UserPassword(roles[0], password)
	limited, err := sqlx.ConnectContext(ctx, "pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { limited.Close() })
	for _, table := range []string{"universities", "groups"} {
		var writable bool
		if err := limited.GetContext(ctx, &writable, `SELECT has_any_column_privilege(current_user,$1,'UPDATE')`, table); err != nil || writable {
			t.Fatalf("bot must not update %s: writable=%t err=%v", table, writable, err)
		}
	}
	university := "subscription-university-" + suffix
	groupA, groupB := university+":a", university+":b"
	cleanupRepositoryUniversity(t, db, university)
	if _, err := db.ExecContext(ctx, `INSERT INTO universities(id,name) VALUES($1,'Test university')`, university); err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{groupA, groupB} {
		if _, err := db.ExecContext(ctx, `INSERT INTO groups(id,university_id,name) VALUES($1,$2,$1)`, group, university); err != nil {
			t.Fatal(err)
		}
	}
	subscriptions := repository.NewSubscriptionRepository(limited)
	users := repository.NewUserRepository(limited)
	for _, scenario := range []string{"subscribe", "confirm_primary", "change_primary", "unsubscribe_primary", "chat_profile"} {
		t.Run(scenario, func(t *testing.T) {
			user := scenario + "-" + suffix
			if _, err := users.CreateUser(ctx, user, "Synthetic user", false); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=$1`, user) })
			if scenario == "change_primary" || scenario == "unsubscribe_primary" {
				for _, group := range []string{groupA, groupB} {
					if _, err := db.ExecContext(ctx, `INSERT INTO subscriptions(id,user_id,object_id,object_type) VALUES($1,$2,$3,'group')`, uuid.NewString(), user, group); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.ExecContext(ctx, `UPDATE users SET default_group_id=$2 WHERE id=$1`, user, groupA); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			switch scenario {
			case "subscribe":
				err = subscriptions.UpsertActiveGroupSubscription(ctx, uuid.NewString(), user, groupA)
			case "confirm_primary":
				err = subscriptions.SubscribeAndSetDefault(ctx, uuid.NewString(), user, groupA)
			case "change_primary":
				err = subscriptions.SetDefaultSubscribedGroup(ctx, user, groupB)
			case "unsubscribe_primary":
				var replacement string
				replacement, err = subscriptions.UnsubscribeAndSelectDefault(ctx, user, groupA)
				if err == nil && replacement != groupB {
					t.Fatalf("replacement=%q, want %q", replacement, groupB)
				}
			case "chat_profile":
				chat := "chat-" + suffix
				t.Cleanup(func() { db.Exec(`DELETE FROM chat_schedule_profiles WHERE chat_id=$1`, chat) })
				err = repository.NewChatProfileRepository(limited).Upsert(ctx, chat, "Synthetic chat", groupA, user)
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "confirm_primary" || scenario == "change_primary" || scenario == "unsubscribe_primary" {
				profile, err := users.GetUserByID(ctx, user)
				want := groupB
				if scenario == "confirm_primary" {
					want = groupA
				}
				if err != nil || profile == nil || profile.DefaultGroupID != want {
					t.Fatalf("primary group not saved: profile=%+v err=%v", profile, err)
				}
			}
		})
	}
	for _, table := range []string{"universities", "groups"} {
		t.Run("concurrent_deactivation_"+table, func(t *testing.T) {
			user := table + "-" + suffix
			if _, err := users.CreateUser(ctx, user, "Synthetic user", false); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Exec(`DELETE FROM users WHERE id=$1`, user) })
			id := groupA
			if table == "universities" {
				id = university
			}
			tx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET is_active=FALSE WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() { finished <- subscriptions.SubscribeAndSetDefault(ctx, uuid.NewString(), user, groupA) }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				select {
				case err := <-finished:
					t.Fatalf("subscription did not wait for deactivation: %v", err)
				default:
				}
				var waiting bool
				if err := db.GetContext(ctx, &waiting, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock')`, roles[0]); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("subscription never reached the row lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-finished; !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("inactive group accepted: %v", err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE `+table+` SET is_active=TRUE WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := database.CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
		t.Fatal(err)
	}
}
