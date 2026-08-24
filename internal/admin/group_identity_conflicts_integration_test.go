//go:build integration

package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestResolveGroupIdentityConflict(t *testing.T) {
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
	universityID := "identity-resolution-university-" + suffix
	sourceID := "identity-resolution-source-" + suffix
	userID := "identity-resolution-user-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM subscriptions WHERE user_id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})

	mustExec := func(label string, query string, args ...any) {
		t.Helper()
		if _, execErr := db.ExecContext(ctx, query, args...); execErr != nil {
			t.Fatalf("%s: %v", label, execErr)
		}
	}
	mustExec("create identity resolution university", `
		INSERT INTO universities (id, name, full_name, schedule_url, is_active)
		VALUES ($1, 'Identity resolution', 'Identity resolution', 'https://example.test', TRUE)`,
		universityID)
	mustExec("create identity resolution source", `
		INSERT INTO data_sources (
			id, university_id, adapter_type, config, update_interval,
			last_error, consecutive_failures, next_retry_at
		) VALUES ($1, $2, 'integration', '{}', 3600, 'identity conflict', 3, NOW()+INTERVAL '1 hour')`,
		sourceID, universityID)
	mustExec("create fixture user", `INSERT INTO users (id) VALUES ($1)`, userID)

	store := NewStore(db)
	t.Run("rename preserves consumers", func(t *testing.T) {
		groupID := "identity-rename-group-" + suffix
		conflictID := "identity-rename-conflict-" + suffix
		mustExec("create rename group", `
			INSERT INTO groups (id, university_id, name, is_active) VALUES ($1,$2,'2-ЭЭ-В',TRUE)`,
			groupID, universityID)
		mustExec("create rename subscription", `
			INSERT INTO subscriptions (id, user_id, object_id, object_type)
			VALUES ($1,$2,$3,'group')`,
			"identity-subscription-"+suffix, userID, groupID)
		mustExec("select rename group as default", `
			UPDATE users SET default_group_id=$1 WHERE id=$2`, groupID, userID)
		mustExec("create rename chat", `
			INSERT INTO chat_schedule_profiles (chat_id, title, default_group_id, configured_by)
			VALUES ($1,'Test chat',$2,$3)`, "identity-chat-"+suffix, groupID, userID)
		mustExec("create rename conflict", `
			INSERT INTO group_identity_conflicts (
				id, data_source_id, university_id, external_group_id,
				existing_group_id, existing_name, incoming_name
			) VALUES ($1,$2,$3,'ispu:group:101016',$4,'2-ЭЭ-В','1-ЭЭ-В')`,
			conflictID, sourceID, universityID, groupID)

		resolved, resolveErr := store.ResolveGroupIdentityConflict(
			ctx, sourceID, conflictID, "rename", "integration-admin",
		)
		if resolveErr != nil {
			t.Fatalf("resolve rename conflict: %v", resolveErr)
		}
		if resolved.ResolvedGroupID == nil || *resolved.ResolvedGroupID != groupID {
			t.Fatalf("resolved rename group = %+v", resolved.ResolvedGroupID)
		}
		var state struct {
			Name          string `db:"name"`
			Subscriptions int    `db:"subscriptions"`
			Defaults      int    `db:"defaults"`
			Chats         int    `db:"chats"`
		}
		if queryErr := db.GetContext(ctx, &state, `
			SELECT group_row.name,
				(SELECT COUNT(*) FROM subscriptions WHERE object_type='group' AND object_id=group_row.id) AS subscriptions,
				(SELECT COUNT(*) FROM users WHERE default_group_id=group_row.id) AS defaults,
				(SELECT COUNT(*) FROM chat_schedule_profiles WHERE default_group_id=group_row.id) AS chats
			FROM groups group_row WHERE group_row.id=$1`, groupID); queryErr != nil {
			t.Fatalf("load renamed group state: %v", queryErr)
		}
		if state.Name != "1-ЭЭ-В" || state.Subscriptions != 1 || state.Defaults != 1 || state.Chats != 1 {
			t.Fatalf("rename did not preserve consumers: %+v", state)
		}
	})

	t.Run("new group preserves old identity", func(t *testing.T) {
		oldGroupID := "identity-old-group-" + suffix
		conflictID := "identity-new-conflict-" + suffix
		mustExec("create old group", `
			INSERT INTO groups (id, university_id, name, is_active) VALUES ($1,$2,'4-А',TRUE)`,
			oldGroupID, universityID)
		mustExec("create new-group conflict", `
			INSERT INTO group_identity_conflicts (
				id, data_source_id, university_id, external_group_id,
				existing_group_id, existing_name, incoming_name
			) VALUES ($1,$2,$3,'source:group:7',$4,'4-А','1-Б')`,
			conflictID, sourceID, universityID, oldGroupID)

		resolved, resolveErr := store.ResolveGroupIdentityConflict(
			ctx, sourceID, conflictID, "new_group", "integration-admin",
		)
		if resolveErr != nil {
			t.Fatalf("resolve new-group conflict: %v", resolveErr)
		}
		if resolved.ResolvedGroupID == nil || *resolved.ResolvedGroupID == oldGroupID {
			t.Fatalf("new resolution reused old group: %+v", resolved.ResolvedGroupID)
		}
		var oldName string
		if queryErr := db.GetContext(ctx, &oldName, `SELECT name FROM groups WHERE id=$1`, oldGroupID); queryErr != nil {
			t.Fatalf("load old group: %v", queryErr)
		}
		if oldName != "4-А" {
			t.Fatalf("old group was changed to %q", oldName)
		}
		var mapping struct {
			GroupID      string `db:"group_id"`
			ExpectedName string `db:"expected_name"`
			IsActive     bool   `db:"is_active"`
		}
		if queryErr := db.GetContext(ctx, &mapping, `
			SELECT mapping.group_id, mapping.expected_name, group_row.is_active
			FROM group_source_identity_mappings mapping
			JOIN groups group_row ON group_row.id=mapping.group_id
			WHERE mapping.data_source_id=$1 AND mapping.external_group_id='source:group:7'`,
			sourceID); queryErr != nil {
			t.Fatalf("load new group mapping: %v", queryErr)
		}
		if mapping.GroupID != *resolved.ResolvedGroupID || mapping.ExpectedName != "1-Б" || mapping.IsActive {
			t.Fatalf("unexpected new group mapping: %+v", mapping)
		}
	})

	var retryState struct {
		LastError           string     `db:"last_error"`
		ConsecutiveFailures int        `db:"consecutive_failures"`
		NextRetryAt         *time.Time `db:"next_retry_at"`
	}
	if err = db.GetContext(ctx, &retryState, `
		SELECT last_error, consecutive_failures, next_retry_at FROM data_sources WHERE id=$1`, sourceID); err != nil {
		t.Fatalf("load source retry state: %v", err)
	}
	if retryState.LastError != "" || retryState.ConsecutiveFailures != 0 || retryState.NextRetryAt != nil {
		t.Fatalf("source retry state was not reset: %+v", retryState)
	}
}
