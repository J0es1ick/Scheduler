//go:build integration

package repository_test

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func TestTimeGridWithRestrictedBotRoleIncludesEntireUniversityDay(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO universities(id,name) VALUES('grid-university','Test university');
		INSERT INTO semesters(id,university_id,external_id,name,start_date,end_date)
		VALUES('grid-term','grid-university','grid-term','Term','2026-09-01','2026-12-31');
		INSERT INTO groups(id,university_id,name) VALUES
		('morning-group','grid-university','Morning'),('afternoon-group','grid-university','Afternoon');
	`); err != nil {
		t.Fatal(err)
	}
	want := []domain.LessonTimeSlot{
		{Start: "08:00", End: "09:35"}, {Start: "09:50", End: "11:25"},
		{Start: "12:10", End: "13:45"}, {Start: "14:00", End: "15:35"},
		{Start: "15:50", End: "17:25"}, {Start: "17:40", End: "19:15"},
	}
	for i, slot := range want {
		group, day := "morning-group", 1
		if i >= 2 && i <= 4 {
			group, day = "afternoon-group", 3
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type)
			VALUES($1,'grid-university','grid-term',$2,$3,$4,$5,'every','Test lesson','lecture')`,
			fmt.Sprintf("grid-lesson-%d", i), group, day, slot.Start, slot.End); err != nil {
			t.Fatal(err)
		}
	}
	var schema string
	if err := db.GetContext(ctx, &schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	role := "grid_bot_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedRole := pgx.Identifier{role}.Sanitize()
	password := uuid.NewString()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+quotedRole+` LOGIN PASSWORD '`+password+`'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DROP OWNED BY ` + quotedRole); err != nil {
			t.Error(err)
		}
		if _, err := db.Exec(`DROP ROLE ` + quotedRole); err != nil {
			t.Error(err)
		}
	})
	for _, statement := range []string{
		`GRANT USAGE ON SCHEMA ` + pgx.Identifier{schema}.Sanitize() + ` TO ` + quotedRole,
		`GRANT SELECT ON effective_lessons, groups, semesters TO ` + quotedRole,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	dsn.User = url.UserPassword(role, password)
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	limited, err := sqlx.ConnectContext(ctx, "pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { limited.Close() })
	var rawAccess bool
	if err := limited.GetContext(ctx, &rawAccess, `SELECT has_table_privilege(current_user,'lessons','SELECT')`); err != nil || rawAccess {
		t.Fatalf("test role must not read raw lessons: access=%t err=%v", rawAccess, err)
	}
	repo := repository.NewLessonRepository(limited)
	from := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for _, days := range []int{1, 7, 14} {
		t.Run(fmt.Sprintf("%d_days", days), func(t *testing.T) {
			slots, err := repo.GetTimeSlots(ctx, "grid-university", from, from.AddDate(0, 0, days-1))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(slots, want) {
				t.Fatalf("grid lost morning or evening slots: got %+v, want %+v", slots, want)
			}
		})
	}
}
