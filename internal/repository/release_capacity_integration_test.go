//go:build integration

package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
)

func TestReleaseCapacityPublishes50000Lessons(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.Exec(`INSERT INTO universities(id,name) VALUES('capacity','Synthetic'); INSERT INTO data_sources(id,university_id,adapter_type) VALUES('capacity-source','capacity','integration')`); err != nil {
		t.Fatal(err)
	}
	logs := repository.NewParseLogRepository(db)
	if _, err := logs.CreateParseLog(ctx, "capacity-log", "capacity-source", "running", 0, ""); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	payload := domain.ScheduleSnapshot{UniversityID: "capacity", SemesterID: "capacity-term", StartDate: start, EndDate: end}
	for g := 0; g < 1000; g++ {
		id := fmt.Sprintf("capacity-group-%d", g)
		group := domain.SnapshotGroup{ID: id, UniversityID: "capacity", Name: fmt.Sprintf("TEST-%04d", g)}
		for l := 0; l < 50; l++ {
			group.Lessons = append(group.Lessons, domain.Lesson{ID: fmt.Sprintf("capacity-lesson-%d-%d", g, l), GroupID: id, UniversityID: "capacity", SemesterID: "capacity-term", DayOfWeek: l%5 + 1, TimeStart: "09:00", TimeEnd: "10:30", WeekType: domain.WeekTypeEvery, Subject: fmt.Sprintf("Synthetic %d", l), Type: domain.LessonTypeLecture, ValidFrom: &start, ValidTo: &end})
		}
		payload.Groups = append(payload.Groups, group)
	}
	snapshots := repository.NewParserSnapshotRepository(db)
	snapshot := &domain.ParserSnapshot{ID: "capacity-snapshot", DataSourceID: "capacity-source", ParseLogID: "capacity-log", Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1000, LessonCount: 50000, Payload: payload}
	if err := snapshots.Create(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	if _, err := snapshots.Publish(ctx, snapshot.ID, "test", "synthetic capacity publication"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT count(*) FROM effective_lessons WHERE university_id='capacity'`); err != nil || count != 50000 {
		t.Fatalf("published count=%d err=%v", count, err)
	}
	t.Logf("Published 1000 groups / 50000 lessons in %s (database publication probe only)", time.Since(begin))
}
