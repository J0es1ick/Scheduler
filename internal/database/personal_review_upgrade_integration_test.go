//go:build integration

package database

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestPersonalReviewUpgradePreservesAndBackfillsOverrides(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "058_personal_schedule.up.sql"); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO universities(id,name) VALUES('u','University');
 INSERT INTO groups(id,university_id,name) VALUES('g','u','Group');
 INSERT INTO semesters(id,external_id,university_id,name,start_date,end_date) VALUES('s','s','u','Semester','2026-09-01','2026-12-31');
 INSERT INTO users(id) VALUES('1');
 INSERT INTO lessons(id,university_id,group_id,semester_id,day_of_week,time_start,time_end,subject,type,week_type,valid_from,valid_to) VALUES('l','u','g','s',1,'09:00','10:30','Physics','lecture','every','2026-09-07','2026-12-28');
 INSERT INTO personal_schedule_overrides(id,user_id,role,target_id,lesson_id,university_id,semester_id,scope,valid_from,valid_to,patch) VALUES('o','1','student','g','l','u','s','semester','2026-09-07','2026-12-31','{"room":"Personal"}');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var item domain.PersonalOverride
	if err = db.Get(&item, `SELECT * FROM personal_schedule_overrides WHERE id='o'`); err != nil {
		t.Fatal(err)
	}
	var basis domain.Lesson
	if err = json.Unmarshal(item.Basis, &basis); err != nil {
		t.Fatal(err)
	}
	if basis.GroupID != "g" || basis.TimeStart != "09:00" || basis.ValidFrom == nil || basis.ValidTo == nil || basis.DayOfWeek != 1 || item.NeedsReview || item.Version != 1 {
		t.Fatalf("upgrade changed override: %+v %+v", item, basis)
	}
	if _, err = db.Exec(`SELECT scheduler_personal_publication_changed('u',ARRAY['g'])`); err != nil {
		t.Fatal(err)
	}
	if err = db.Get(&item, `SELECT * FROM personal_schedule_overrides WHERE id='o'`); err != nil {
		t.Fatal(err)
	}
	if !item.NeedsReview || item.Version != 2 {
		t.Fatal("review not requested", item)
	}
}
