//go:build integration

package database

import (
	"context"
	"os"
	"testing"
)

func TestTelegramUXFeedbackMigration(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "051_ambiguous_historical_ingestion.up.sql"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users(id) VALUES('student'); INSERT INTO support_requests(id,user_id,request_type,details) VALUES('old','student','update_existing','Existing request must remain intact');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO support_requests(id,user_id,request_type,details) VALUES('new','student','feedback','General feedback before migration')`); err == nil {
		t.Fatal("051 unexpectedly accepts feedback")
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO support_requests(id,user_id,request_type,details) VALUES('new','student','feedback','General feedback after migration')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM support_requests`); err != nil || count != 2 {
		t.Fatalf("migration changed existing requests: count=%d err=%v", count, err)
	}
}
