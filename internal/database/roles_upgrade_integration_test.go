//go:build integration

package database

import (
	"context"
	"os"
	"testing"
)

func TestUpgradeRetainsStudentProfilesWithoutDailyPrompt(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "055_privacy_deletion_lock_order.up.sql"); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO universities(id,name) VALUES('role-upgrade','Role upgrade');
 INSERT INTO groups(id,name,university_id) VALUES('role-upgrade','Group','role-upgrade');
 INSERT INTO users(id,notifications_enabled) VALUES('complete',FALSE),('incomplete',TRUE);
 INSERT INTO subscriptions(id,user_id,object_id,object_type,subgroup,schedule_view_format) VALUES('role-upgrade','complete','role-upgrade','group',2,'compact');
 UPDATE users SET default_group_id='role-upgrade' WHERE id='complete'`)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var valid bool
	err = db.Get(&valid, `SELECT
 EXISTS(SELECT 1 FROM users WHERE id='complete' AND role='student' AND daily_setup='done' AND NOT daily_enabled AND NOT notifications_enabled AND default_group_id='role-upgrade')
 AND EXISTS(SELECT 1 FROM users WHERE id='incomplete' AND role='student' AND daily_setup='choice' AND NOT daily_enabled)
 AND EXISTS(SELECT 1 FROM subscriptions WHERE user_id='complete' AND subgroup=2 AND schedule_view_format='compact')`)
	if err != nil || !valid {
		t.Fatalf("migration lost profile settings: %t %v", valid, err)
	}
}
