//go:build integration

package admin

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/parserruntime"
	"github.com/jmoiron/sqlx"
)

func TestReleaseBrowserFixture(t *testing.T) {
	if os.Getenv("RELEASE_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in isolated browser fixture")
	}
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if dsn.Hostname() != "127.0.0.1" || !strings.HasSuffix(dsn.Path, "_browser_test") {
		t.Fatal("fixture requires a disposable loopback *_browser_test database")
	}
	db, err := sqlx.Connect("pgx", dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO universities(id,name,full_name,timezone) VALUES('release-test','Тестовый вуз','Изолированный стенд Scheduler','Europe/Moscow') ON CONFLICT DO NOTHING;
 INSERT INTO groups(id,university_id,name) VALUES('release-group','release-test','ТЕСТ-101') ON CONFLICT DO NOTHING;
 INSERT INTO semesters(id,external_id,university_id,name,start_date,end_date) VALUES('release-term','release-term','release-test','Осень 2026','2026-09-01','2026-12-31') ON CONFLICT DO NOTHING;
 INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,valid_from,valid_to,recurrence) VALUES('release-lesson','release-test','release-term','release-group',3,'09:00','10:30','every','Тестовая физика','lecture','2026-09-02','2026-12-31','{"cycle_length":3,"cycle_weeks":[1,3],"anchor_date":"2026-09-01T00:00:00Z"}') ON CONFLICT DO NOTHING;`)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	auth := NewAuthManager("", "release-browser-fixture-key", true, false)
	auth.UseSessionStore(store)
	server, err := NewServer(store, auth, parserruntime.New(db))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	defer httpServer.Close()
	go httpServer.Serve(listener)
	t.Log("Synthetic browser fixture ready on 127.0.0.1:18080 for up to 15 minutes")
	<-time.After(15 * time.Minute)
}
