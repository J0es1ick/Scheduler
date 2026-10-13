//go:build integration

package database

import (
	"context"
	"os"
	"testing"
)

func TestBroadcastUpgradePreservesExistingAudiences(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "059_personal_schedule_review.up.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id,service_updates_consent) VALUES('42',TRUE),('43',FALSE),('44',NULL);
		INSERT INTO broadcasts(id,audience_mode,status) VALUES('updates','all','sending'),('selected','selected','draft');
		INSERT INTO bot_outbox(id,user_id,kind,body,broadcast_id) VALUES('old','42','service_update','Updates','updates');`); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var valid bool
	if err := db.Get(&valid, `SELECT
		EXISTS(SELECT 1 FROM broadcasts WHERE id='updates' AND audience_mode='all' AND status='sending') AND
		EXISTS(SELECT 1 FROM broadcasts WHERE id='selected' AND audience_mode='selected' AND status='draft') AND
		EXISTS(SELECT 1 FROM notification_queue_eligibility WHERE id='old' AND policy_decision='ready') AND
		EXISTS(SELECT 1 FROM users WHERE id='43' AND service_updates_consent IS FALSE) AND
		EXISTS(SELECT 1 FROM users WHERE id='44' AND service_updates_consent IS NULL)`); err != nil || !valid {
		t.Fatalf("upgrade changed existing data: %t %v", valid, err)
	}
	if _, err := db.Exec(`INSERT INTO broadcasts(id,audience_mode,status) VALUES('important','all_users','sending');
		INSERT INTO bot_outbox(id,user_id,kind,body,broadcast_id) VALUES('new','42','service_update','Maintenance','important');
		UPDATE users SET service_updates_consent=FALSE WHERE id='42';`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&valid, `SELECT
		EXISTS(SELECT 1 FROM bot_outbox WHERE id='old' AND status='cancelled') AND
		EXISTS(SELECT 1 FROM notification_queue_eligibility WHERE id='new' AND policy_decision='ready')`); err != nil || !valid {
		t.Fatalf("wrong consent policy after upgrade: %t %v", valid, err)
	}
}
