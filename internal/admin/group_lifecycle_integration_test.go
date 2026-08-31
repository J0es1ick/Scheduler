//go:build integration

package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestGroupLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	suffix := uuid.NewString()
	universityID := "group-lifecycle-university-" + suffix
	groupID := "group-lifecycle-group-" + suffix
	oldGroupID := "group-lifecycle-old-group-" + suffix
	userID := "group-lifecycle-user-" + suffix
	chatID := "group-lifecycle-chat-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})

	mustExec := func(label, query string, args ...any) {
		t.Helper()
		if _, execErr := db.ExecContext(ctx, query, args...); execErr != nil {
			t.Fatalf("%s: %v", label, execErr)
		}
	}
	mustExec("create university", `
		INSERT INTO universities (id, name, full_name, schedule_url, is_active)
		VALUES ($1,'Lifecycle university','Lifecycle university','https://example.test',TRUE)`,
		universityID)
	mustExec("create group", `
		INSERT INTO groups (
			id, university_id, name, is_active, source_active, manually_disabled
		) VALUES ($1,$2,'TEST-1',TRUE,TRUE,FALSE)`, groupID, universityID)
	mustExec("create older inactive group", `
		INSERT INTO groups (
			id, university_id, name, is_active, source_active,
			manually_disabled, created_at, updated_at
		) VALUES ($1,$2,'TEST-OLD',FALSE,FALSE,FALSE,NOW()-INTERVAL '1 year',NOW())`,
		oldGroupID, universityID)
	mustExec("create user", `
		INSERT INTO users (id) VALUES ($1)`, userID)
	mustExec("create subscription", `
		INSERT INTO subscriptions (id, user_id, object_id, object_type)
		VALUES ($1,$2,$3,'group')`, "group-lifecycle-subscription-"+suffix, userID, groupID)
	mustExec("set default", `UPDATE users SET default_group_id=$2 WHERE id=$1`, userID, groupID)
	mustExec("create chat", `
		INSERT INTO chat_schedule_profiles (chat_id, title, default_group_id, configured_by)
		VALUES ($1,'Lifecycle chat',$2,$3)`, chatID, groupID, userID)

	store := NewStore(db)
	oldest, listErr := store.Groups(ctx, 1, 10, universityID, "", "all", "oldest", false)
	if listErr != nil {
		t.Fatalf("list oldest groups: %v", listErr)
	}
	if len(oldest.Items) != 2 || oldest.Items[0].ID != oldGroupID {
		t.Fatalf("oldest group order = %+v", oldest.Items)
	}
	newest, listErr := store.Groups(ctx, 1, 10, universityID, "", "all", "newest", false)
	if listErr != nil {
		t.Fatalf("list newest groups: %v", listErr)
	}
	if len(newest.Items) != 2 || newest.Items[0].ID != groupID {
		t.Fatalf("newest group order = %+v", newest.Items)
	}
	if _, err = store.DeleteGroup(ctx, groupID); !errors.Is(err, ErrGroupActive) {
		t.Fatalf("delete active group error = %v, want ErrGroupActive", err)
	}

	group, err := store.SetGroupActive(ctx, groupID, false)
	if err != nil {
		t.Fatalf("deactivate group: %v", err)
	}
	if group.IsActive || !group.SourceActive || !group.ManuallyDisabled {
		t.Fatalf("unexpected deactivated group: %+v", group)
	}
	if group.SubscriptionCount != 1 || group.DefaultGroupCount != 1 || group.ChatCount != 1 {
		t.Fatalf("deactivation did not preserve consumers: %+v", group)
	}

	mustExec("simulate source publication", `
		UPDATE groups SET source_active=TRUE, is_active=NOT manually_disabled WHERE id=$1`, groupID)
	group, err = store.Group(ctx, groupID)
	if err != nil {
		t.Fatalf("load group after publication: %v", err)
	}
	if group.IsActive {
		t.Fatal("parser publication reactivated a manually disabled group")
	}

	group, err = store.SetGroupActive(ctx, groupID, true)
	if err != nil {
		t.Fatalf("reactivate group: %v", err)
	}
	if !group.IsActive || group.ManuallyDisabled {
		t.Fatalf("unexpected reactivated group: %+v", group)
	}

	if _, err = store.SetGroupActive(ctx, groupID, false); err != nil {
		t.Fatalf("deactivate group before archival: %v", err)
	}
	mustExec("mark group absent from source", `
		UPDATE groups SET source_active=FALSE, is_active=FALSE WHERE id=$1`, groupID)
	deleted, err := store.DeleteGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("delete inactive group: %v", err)
	}
	if deleted.SubscriptionCount != 1 || deleted.DefaultGroupCount != 1 || deleted.ChatCount != 1 {
		t.Fatalf("unexpected deletion summary: %+v", deleted)
	}

	var groupCount, subscriptionCount, chatCount, defaultCount int
	if err = db.GetContext(ctx, &groupCount, `SELECT COUNT(*)::int FROM groups WHERE id=$1`, groupID); err != nil {
		t.Fatalf("count deleted group: %v", err)
	}
	if err = db.GetContext(ctx, &subscriptionCount, `
		SELECT COUNT(*)::int FROM subscriptions WHERE object_type='group' AND object_id=$1`, groupID); err != nil {
		t.Fatalf("count deleted subscriptions: %v", err)
	}
	if err = db.GetContext(ctx, &chatCount, `
		SELECT COUNT(*)::int FROM chat_schedule_profiles WHERE default_group_id=$1`, groupID); err != nil {
		t.Fatalf("count deleted chats: %v", err)
	}
	if err = db.GetContext(ctx, &defaultCount, `
		SELECT COUNT(*)::int FROM users WHERE id=$1 AND default_group_id IS NOT NULL`, userID); err != nil {
		t.Fatalf("count retained defaults: %v", err)
	}
	if groupCount != 0 || subscriptionCount != 0 || chatCount != 0 || defaultCount != 0 {
		t.Fatalf(
			"group deletion left related data: group=%d subscriptions=%d chats=%d defaults=%d",
			groupCount, subscriptionCount, chatCount, defaultCount,
		)
	}
}

func TestGroupDeletionAndUnsubscribeDoNotDeadlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM users WHERE id LIKE $1`, suffix+"%")
		db.ExecContext(context.Background(), `DELETE FROM universities WHERE id=$1`, suffix)
	})
	if _, err = db.ExecContext(ctx, `INSERT INTO universities (id,name,full_name,schedule_url) VALUES ($1,$1,$1,'')`, suffix); err != nil {
		t.Fatal(err)
	}
	for round := range 20 {
		id := suffix + fmt.Sprint(round)
		if _, err = db.ExecContext(ctx, `INSERT INTO groups (id,university_id,name,is_active,source_active) VALUES ($1,$2,$1,FALSE,FALSE)`, id, suffix); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO users (id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `INSERT INTO subscriptions (id,user_id,object_id,object_type) VALUES ($1,$1,$1,'group')`, id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, `UPDATE users SET default_group_id=$1 WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Go(func() { <-start; _, err := NewStore(db).DeleteGroup(ctx, id); results <- err })
		wg.Go(func() {
			<-start
			_, err := repository.NewSubscriptionRepository(db).UnsubscribeAndSelectDefault(ctx, id, id)
			results <- err
		})
		close(start)
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatal(err)
			}
		}
		if err = database.CheckSubscriptionIntegrity(ctx, db.DB); err != nil {
			t.Fatal(err)
		}
	}
}
