//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
)

func TestDraftDoesNotChangeLiveInstitution(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.Exec(`INSERT INTO universities(id,name,full_name,schedule_url,timezone,locale) VALUES ('live','Live','Live university','https://live.example','Europe/Moscow','ru-RU')`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewConnectorRepository(db)
	if _, err := repo.Create(ctx, repository.CreateConnectorParams{ConnectorID: "draft", SourceID: "draft-source", UniversityID: "live", UniversityName: "Proposed", UniversityFullName: "Proposed university", ScheduleURL: "https://proposed.example", Timezone: "Asia/Tokyo", Locale: "en-US", DisplayName: "Draft", KeyID: "key", PublicKey: []byte("synthetic"), CreatedBy: "test", QualityPolicy: domain.DefaultSourceQualityPolicy()}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{domain.ConnectorStatusTesting, domain.ConnectorStatusArchived} {
		if err := repo.UpdateStatus(ctx, "draft", status); err != nil {
			t.Fatal(err)
		}
		var unchanged bool
		if err := db.Get(&unchanged, `SELECT name='Live' AND full_name='Live university' AND schedule_url='https://live.example' AND timezone='Europe/Moscow' AND locale='ru-RU' FROM universities WHERE id='live'`); err != nil || !unchanged {
			t.Fatalf("draft mutated live institution: %t %v", unchanged, err)
		}
	}
	var timezone string
	if err := db.Get(&timezone, `SELECT config::jsonb->'candidate_institution'->>'timezone' FROM data_sources WHERE id='draft-source'`); err != nil || timezone != "Asia/Tokyo" {
		t.Fatalf("candidate lost metadata: %q %v", timezone, err)
	}
}

func TestRetryCannotOverwriteNewerPublication(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	runs := repository.NewConnectorRepository(db)
	if _, err := runs.Create(ctx, repository.CreateConnectorParams{ConnectorID: "connector", SourceID: "source", UniversityID: "university", UniversityName: "University", DisplayName: "External", KeyID: "key", PublicKey: []byte("synthetic"), CreatedBy: "test", QualityPolicy: domain.DefaultSourceQualityPolicy()}); err != nil {
		t.Fatal(err)
	}
	if err := runs.UpdateStatus(ctx, "connector", domain.ConnectorStatusActive); err != nil {
		t.Fatal(err)
	}
	a, _, err := runs.Enqueue(ctx, "connector", "A", "1.0", "A", "digest-a", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := runs.Enqueue(ctx, "connector", "B", "1.0", "B", "digest-b", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.IngestionSequence <= 0 || b.IngestionSequence <= a.IngestionSequence {
		t.Fatalf("invalid reception order A=%d B=%d", a.IngestionSequence, b.IngestionSequence)
	}
	replay, duplicate, err := runs.Enqueue(ctx, "connector", "A", "1.0", "A", "digest-a", []byte(`{}`))
	if err != nil || !duplicate || replay.IngestionSequence != a.IngestionSequence {
		t.Fatalf("idempotency changed reception order: %v", err)
	}
	claim, err := runs.ClaimNext(ctx)
	if err != nil || claim == nil || claim.ID != a.ID {
		t.Fatalf("claim A: %+v %v", claim, err)
	}
	if err = runs.Fail(ctx, claim.ID, claim.ClaimToken, errors.New("synthetic temporary failure"), true); err != nil {
		t.Fatal(err)
	}
	claim, err = runs.ClaimNext(ctx)
	if err != nil || claim == nil || claim.ID != b.ID {
		t.Fatalf("claim B: %+v %v", claim, err)
	}
	groups := repository.NewGroupRepository(db)
	parser := service.NewParserService(repository.NewDataSourceRepository(db), repository.NewParseLogRepository(db), groups, service.NewScheduleService(repository.NewLessonRepository(db), repository.NewSemesterRepository(db), groups), repository.NewParserSnapshotRepository(db), repository.NewNotificationRepository(db), repository.NewParserDiagnosticRepository(db))
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 4, 0)
	payload := func(seq int64, room string) domain.ScheduleSnapshot {
		return domain.ScheduleSnapshot{IngestionSequence: seq, UniversityID: "university", SemesterID: "term", StartDate: start, EndDate: end, Groups: []domain.SnapshotGroup{{ID: "group", Name: "TEST-1", UniversityID: "university", Lessons: []domain.Lesson{{ID: "lesson", ExternalID: "lesson", GroupID: "group", UniversityID: "university", SemesterID: "term", DayOfWeek: 3, WeekType: domain.WeekTypeEvery, TimeStart: "09:00", TimeEnd: "10:30", Subject: "Physics", Type: domain.LessonTypeLecture, Room: room, ValidFrom: &start, ValidTo: &end}}}}}
	}
	published, err := parser.IngestClaimedExternalSnapshot(ctx, "source", payload(b.IngestionSequence, "B"), b.ID, claim.ClaimToken)
	if err != nil || published.Status != domain.SnapshotStatusPublished {
		t.Fatalf("publish B: %+v %v", published, err)
	}
	if _, err = db.Exec(`UPDATE connector_ingestion_runs SET next_attempt_at=NOW() WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	claim, err = runs.ClaimNext(ctx)
	if err != nil || claim == nil || claim.ID != a.ID {
		t.Fatalf("retry A: %+v %v", claim, err)
	}
	if _, err = parser.IngestClaimedExternalSnapshot(ctx, "source", payload(a.IngestionSequence, "A"), a.ID, claim.ClaimToken); !errors.Is(err, repository.ErrConnectorSuperseded) {
		t.Fatalf("old publication accepted: %v", err)
	}
	if err = runs.Complete(ctx, a.ID, claim.ClaimToken, domain.IngestionStatusSuperseded, "", 0, 0); err != nil {
		t.Fatal(err)
	}
	var room string
	if err = db.Get(&room, `SELECT room FROM effective_lessons WHERE group_id='group'`); err != nil || room != "B" {
		t.Fatalf("new publication lost: room=%s err=%v", room, err)
	}
	var watermark int64
	if err = db.Get(&watermark, `SELECT last_published_ingestion_sequence FROM data_sources WHERE id='source'`); err != nil || watermark != b.IngestionSequence {
		t.Fatalf("watermark=%d err=%v", watermark, err)
	}
}

func TestConcurrentReceptionSerializesOnSource(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	repo := repository.NewConnectorRepository(db)
	if _, err := repo.Create(ctx, repository.CreateConnectorParams{ConnectorID: "parallel", SourceID: "parallel-source", UniversityID: "parallel-university", UniversityName: "Synthetic", DisplayName: "Synthetic", KeyID: "key", PublicKey: []byte("synthetic"), CreatedBy: "test", QualityPolicy: domain.DefaultSourceQualityPolicy()}); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	barrier := make(chan struct{})
	results := make(chan *domain.ConnectorIngestionRun, 12)
	failures := make(chan error, 12)
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM data_sources WHERE id='parallel-source' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		go func(i int) {
			<-barrier
			run, _, err := repo.Enqueue(context.Background(), "parallel", fmt.Sprint(i), "1.0", fmt.Sprint(i), fmt.Sprint(i), []byte(`{}`))
			if err != nil {
				failures <- err
				return
			}
			results <- run
		}(i)
	}
	close(barrier)
	select {
	case <-results:
		t.Fatal("reception escaped source lock")
	case err := <-failures:
		t.Fatal(err)
	case <-time.After(75 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for i := 0; i < 12; i++ {
		select {
		case run := <-results:
			if run.IngestionSequence <= 0 || seen[run.IngestionSequence] {
				t.Fatal("non-unique reception sequence")
			}
			seen[run.IngestionSequence] = true
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent reception stuck")
		}
	}
	for i := int64(1); i <= 12; i++ {
		if !seen[i] {
			t.Fatalf("missing sequence %d", i)
		}
	}
}
