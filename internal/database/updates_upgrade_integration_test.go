//go:build integration

package database

import (
	"context"
	"os"
	"testing"
)

func TestUpdatesUpgradeKeepsConsentUnknownAndBackfillsOnlyExistingUsers(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "056_roles_daily_schedule.up.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id,daily_setup,daily_enabled,daily_time) VALUES('old','done',FALSE,'06:30')`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id) VALUES('new')`); err != nil {
		t.Fatal(err)
	}
	var valid bool
	err := db.Get(&valid, `SELECT EXISTS(SELECT 1 FROM users WHERE id='old' AND service_updates_consent IS NULL AND service_updates_backfill AND service_updates_prompt_delivered_at IS NULL AND daily_setup='done' AND daily_time='06:30') AND EXISTS(SELECT 1 FROM users WHERE id='new' AND service_updates_consent IS NULL AND NOT service_updates_backfill AND length(service_updates_prompt_key)=32)`)
	if err != nil || !valid {
		t.Fatalf("upgrade: %t %v", valid, err)
	}
}
