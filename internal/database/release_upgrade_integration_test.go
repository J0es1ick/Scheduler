//go:build integration

package database

import (
	"context"
	"os"
	"testing"
)

func TestUpgradeRepairsOnlyUnambiguousOverrides(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "044_execute_privacy_deletion.up.sql"); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO universities(id,name) VALUES('repair','Repair');
 INSERT INTO groups(id,name,university_id) VALUES('repair','Repair','repair');
 INSERT INTO semesters(id,external_id,name,university_id,start_date,end_date) VALUES('repair','repair','Term','repair','2026-09-01','2026-12-31');
 INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,recurrence)
 SELECT id,'repair','repair','repair',3,'09:00','10:30','every','Physics','lecture','{"cycle_length":3,"cycle_weeks":[1,3],"anchor_date":"2026-09-01T00:00:00Z"}' FROM (VALUES('same'),('changed')) t(id);
 INSERT INTO lesson_overrides(id,base_lesson_id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,created_by)
 SELECT id||'-override',id,university_id,semester_id,group_id,CASE WHEN id='changed' THEN 4 ELSE day_of_week END,time_start,time_end,week_type,subject,type,'synthetic' FROM lessons WHERE university_id='repair';`)
	if err != nil {
		t.Fatal(err)
	}
	report, readErr := os.ReadFile("../../scripts/release-data-audit.sql")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if _, err = db.Exec(string(report)); err != nil {
		t.Fatalf("read-only report incompatible with 044: %v", err)
	}
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var repaired, ambiguous bool
	if err = db.QueryRowx(`SELECT (SELECT recurrence<>'{}'::jsonb FROM lesson_overrides WHERE id='same-override'),(SELECT recurrence='{}'::jsonb FROM lesson_overrides WHERE id='changed-override')`).Scan(&repaired, &ambiguous); err != nil || !repaired || !ambiguous {
		t.Fatalf("unsafe recovery: repaired=%t ambiguous=%t err=%v", repaired, ambiguous, err)
	}
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("repeat migration failed: %v", err)
	}
}

func TestUpgradeFreezesAmbiguousHistoricalReception(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "044_execute_privacy_deletion.up.sql"); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO universities(id,name) VALUES('history','History');
 INSERT INTO data_sources(id,university_id,adapter_type) VALUES('history','history','external_push');
 INSERT INTO connector_clients(id,data_source_id,display_name,key_id,public_key,created_by) VALUES('history','history','History','history','synthetic','test');
 INSERT INTO connector_ingestion_runs(id,connector_id,external_snapshot_id,schema_version,idempotency_key,payload_sha256,payload,received_at)
 SELECT id,'history',id,'1.0',id,id,'{}','2026-01-01T00:00:00Z'::timestamptz FROM (VALUES('A'),('B')) t(id);`)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var watermark, next int64
	if err = db.QueryRowx(`SELECT last_published_ingestion_sequence,next_ingestion_sequence FROM data_sources WHERE id='history'`).Scan(&watermark, &next); err != nil || watermark != 2 || next != 2 {
		t.Fatalf("ambiguous historical jobs remain publishable: watermark=%d next=%d err=%v", watermark, next, err)
	}
	if _, err = db.Exec(`INSERT INTO connector_ingestion_runs(id,connector_id,external_snapshot_id,schema_version,idempotency_key,payload_sha256,payload) VALUES('fresh','history','fresh','1.0','fresh','fresh','{}')`); err != nil {
		t.Fatal(err)
	}
	var fresh int64
	if err = db.Get(&fresh, `SELECT ingestion_sequence FROM connector_ingestion_runs WHERE id='fresh'`); err != nil || fresh <= watermark {
		t.Fatalf("fresh snapshot cannot clear fence: %d %v", fresh, err)
	}
}
