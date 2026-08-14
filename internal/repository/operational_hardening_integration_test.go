//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func openOperationalIntegrationDB(t *testing.T) (*sqlx.DB, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect integration database: %v", err)
	}
	if err = database.ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		cancel()
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		cancel()
	})
	return db, ctx
}

func TestConnectorRateLimitIsAtomic(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	connectorID := "rate-connector-" + suffix
	universityID := "rate-university-" + suffix
	repo := repository.NewConnectorRepository(db)
	if _, err := repo.Create(ctx, repository.CreateConnectorParams{
		ConnectorID:    connectorID,
		SourceID:       "rate-source-" + suffix,
		UniversityID:   universityID,
		UniversityName: "Rate limit integration",
		DisplayName:    "Rate limit integration",
		KeyID:          "rate-key-" + suffix,
		PublicKey:      []byte("integration-public-key"),
		CreatedBy:      "integration",
		QualityPolicy:  domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})

	const limit = 5
	const callers = 24
	start := make(chan struct{})
	results := make(chan error, callers)
	var group sync.WaitGroup
	for index := range callers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results <- repo.UseNonce(
				ctx,
				connectorID,
				fmt.Sprintf("nonce-%02d-%s", index, suffix),
				time.Now().Add(5*time.Minute),
				limit,
			)
		}(index)
	}
	close(start)
	group.Wait()
	close(results)

	succeeded := 0
	rateLimited := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, repository.ErrConnectorRateLimit):
			rateLimited++
		default:
			t.Fatalf("unexpected nonce result: %v", err)
		}
	}
	if succeeded != limit || rateLimited != callers-limit {
		t.Fatalf("successful requests=%d rate-limited=%d, want %d/%d",
			succeeded, rateLimited, limit, callers-limit)
	}
}

func TestParserFinalizationRollsBackOnSourceUpdateFailure(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	universityID := "finalize-university-" + suffix
	sourceID := "finalize-source-" + suffix
	logID := "finalize-log-" + suffix
	functionName := "integration_reject_source_update_" + strings.ReplaceAll(suffix, "-", "_")
	triggerName := "integration_reject_source_update_" + strings.ReplaceAll(suffix, "-", "_")

	universities := repository.NewUniversityRepository(db)
	if _, err := universities.CreateUniversity(ctx, universityID, "Finalize integration", "", "", true); err != nil {
		t.Fatal(err)
	}
	dataSources := repository.NewDataSourceRepository(db)
	if _, err := dataSources.CreateDataSource(ctx, sourceID, universityID, "integration", "{}", 3600); err != nil {
		t.Fatal(err)
	}
	parseLogs := repository.NewParseLogRepository(db)
	if _, err := parseLogs.CreateParseLog(ctx, logID, sourceID, "running", 0, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON data_sources`, triggerName))
		_, _ = db.ExecContext(cleanupCtx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'injected source update failure';
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s BEFORE UPDATE ON data_sources
		FOR EACH ROW WHEN (OLD.id = '%s') EXECUTE FUNCTION %s()`,
		functionName, triggerName, sourceID, functionName)); err != nil {
		t.Fatalf("install finalization fault: %v", err)
	}

	if _, _, err := parseLogs.FinalizeFailure(ctx, logID, sourceID, 12, "upstream failed"); err == nil {
		t.Fatal("injected source update failure was hidden")
	}
	var state struct {
		Status     string     `db:"status"`
		FinishedAt *time.Time `db:"finished_at"`
		Failures   int        `db:"consecutive_failures"`
		LastError  string     `db:"last_error"`
	}
	if err := db.GetContext(ctx, &state, `
		SELECT log.status, log.finished_at, source.consecutive_failures,
		       COALESCE(source.last_error, '') AS last_error
		FROM parse_logs log
		JOIN data_sources source ON source.id=log.data_source_id
		WHERE log.id=$1`, logID); err != nil {
		t.Fatal(err)
	}
	if state.Status != "running" || state.FinishedAt != nil || state.Failures != 0 || state.LastError != "" {
		t.Fatalf("failed transaction left partial parser state: %+v", state)
	}

	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TRIGGER %s ON data_sources`, triggerName)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP FUNCTION %s()`, functionName)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := parseLogs.FinalizeFailure(ctx, logID, sourceID, 12, "upstream failed"); err != nil {
		t.Fatalf("finalize after removing fault: %v", err)
	}
}

func TestDegradedSourceRemainsVisibleDuringBackoff(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	universityID := "degraded-university-" + suffix
	sourceID := "degraded-source-" + suffix

	universities := repository.NewUniversityRepository(db)
	if _, err := universities.CreateUniversity(ctx, universityID, "Degraded integration", "", "", true); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewDataSourceRepository(db)
	if _, err := repo.CreateDataSource(ctx, sourceID, universityID, "integration", "{}", 3600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})
	if _, err := db.ExecContext(ctx, `
		UPDATE data_sources
		SET is_enabled=TRUE, lifecycle_status='active', last_error='upstream unavailable',
		    next_retry_at=NOW()+INTERVAL '1 hour'
		WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}

	runnable, err := repo.ListActiveDataSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range runnable {
		if source.ID == sourceID {
			t.Fatal("source in backoff was returned as runnable")
		}
	}
	degraded, err := repo.ActiveDegradationCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if degraded < 1 {
		t.Fatal("persisted source failure disappeared from readiness during backoff")
	}
}

func TestOperationalRetentionBoundsPayloadsAndHistory(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	universityID := "retention-university-" + suffix
	sourceID := "retention-source-" + suffix
	connectorID := "retention-connector-" + suffix
	logID := "retention-log-" + suffix
	snapshotID := "retention-snapshot-" + suffix
	auditID := "retention-audit-" + suffix
	recentRunID := "retention-recent-run-" + suffix
	oldRunID := "retention-old-run-" + suffix

	connectors := repository.NewConnectorRepository(db)
	if _, err := connectors.Create(ctx, repository.CreateConnectorParams{
		ConnectorID:    connectorID,
		SourceID:       sourceID,
		UniversityID:   universityID,
		UniversityName: "Retention integration",
		DisplayName:    "Retention integration",
		KeyID:          "retention-key-" + suffix,
		PublicKey:      []byte("integration-public-key"),
		CreatedBy:      "integration",
		QualityPolicy:  domain.DefaultSourceQualityPolicy(),
	}); err != nil {
		t.Fatal(err)
	}
	parseLogs := repository.NewParseLogRepository(db)
	if _, err := parseLogs.CreateParseLog(ctx, logID, sourceID, "failed", 0, "old failure"); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewParserSnapshotRepository(db).Create(ctx, &domain.ParserSnapshot{
		ID: snapshotID, DataSourceID: sourceID, ParseLogID: logID,
		Status: domain.SnapshotStatusRejected, Publishable: false,
		Payload: domain.ScheduleSnapshot{UniversityID: universityID},
	}); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO connector_ingestion_runs
		  (id, connector_id, external_snapshot_id, schema_version, idempotency_key,
		   payload_sha256, payload, status, received_at, completed_at)
		  VALUES ($1,$2,$3,'schedule-snapshot.v1',$4,'digest','{"private":"payload"}',
		          'staged',NOW()-INTERVAL '31 days',NOW()-INTERVAL '31 days')`,
			[]any{recentRunID, connectorID, "recent-" + suffix, "recent-key-" + suffix}},
		{`INSERT INTO connector_ingestion_runs
		  (id, connector_id, external_snapshot_id, schema_version, idempotency_key,
		   payload_sha256, payload, status, received_at, completed_at)
		  VALUES ($1,$2,$3,'schedule-snapshot.v1',$4,'digest','{"private":"payload"}',
		          'failed',NOW()-INTERVAL '181 days',NOW()-INTERVAL '181 days')`,
			[]any{oldRunID, connectorID, "old-" + suffix, "old-key-" + suffix}},
		{`UPDATE parse_logs
		  SET started_at=NOW()-INTERVAL '181 days', finished_at=NOW()-INTERVAL '181 days'
		  WHERE id=$1`, []any{logID}},
		{`UPDATE parser_snapshots SET created_at=NOW()-INTERVAL '91 days' WHERE id=$1`, []any{snapshotID}},
		{`INSERT INTO admin_audit_logs
		  (id, actor_id, actor_name, action, object_type, created_at)
		  VALUES ($1,'integration','Integration','retention','test',NOW()-INTERVAL '366 days')`,
			[]any{auditID}},
		{`UPDATE operational_maintenance SET last_run_at='-infinity' WHERE task_name='retention'`, nil},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM admin_audit_logs WHERE id=$1`, auditID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})

	ran, err := parseLogs.RunOperationalRetention(ctx)
	if err != nil {
		t.Fatalf("run retention: %v", err)
	}
	if !ran {
		t.Fatal("due retention pass was not claimed")
	}
	var recentPayload string
	if err := db.GetContext(ctx, &recentPayload,
		`SELECT payload::text FROM connector_ingestion_runs WHERE id=$1`, recentRunID); err != nil {
		t.Fatal(err)
	}
	if recentPayload != "{}" {
		t.Fatalf("31-day connector payload was retained: %s", recentPayload)
	}
	for name, query := range map[string]string{
		"old connector run": `SELECT EXISTS (SELECT 1 FROM connector_ingestion_runs WHERE id=$1)`,
		"rejected snapshot": `SELECT EXISTS (SELECT 1 FROM parser_snapshots WHERE id=$1)`,
		"orphan parse log":  `SELECT EXISTS (SELECT 1 FROM parse_logs WHERE id=$1)`,
		"old audit entry":   `SELECT EXISTS (SELECT 1 FROM admin_audit_logs WHERE id=$1)`,
	} {
		id := map[string]string{
			"old connector run": oldRunID,
			"rejected snapshot": snapshotID,
			"orphan parse log":  logID,
			"old audit entry":   auditID,
		}[name]
		var exists bool
		if err := db.GetContext(ctx, &exists, query, id); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("retention kept %s", name)
		}
	}
	if ran, err = parseLogs.RunOperationalRetention(ctx); err != nil || ran {
		t.Fatalf("retention ran twice inside 24 hours: ran=%t err=%v", ran, err)
	}
}

func TestDeleteUserRevokesSessionsAndAnonymizesOperationalReferences(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	userID := "privacy-user-" + suffix
	otherUserID := "privacy-owner-" + suffix
	universityID := "privacy-university-" + suffix
	groupID := "privacy-group-" + suffix
	semesterID := "privacy-semester-" + suffix
	sourceID := "privacy-source-" + suffix
	logID := "privacy-log-" + suffix
	snapshotID := "privacy-snapshot-" + suffix
	connectorID := "privacy-connector-" + suffix
	auditID := "privacy-audit-" + suffix
	overrideID := "privacy-override-" + suffix
	requestID := "privacy-request-" + suffix

	users := repository.NewUserRepository(db)
	if _, err := users.CreateUser(ctx, userID, "privacy_target", true); err != nil {
		t.Fatal(err)
	}
	if _, err := users.CreateUser(ctx, otherUserID, "privacy_owner", false); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewUniversityRepository(db).CreateUniversity(
		ctx, universityID, "Privacy integration", "", "", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewGroupRepository(db).CreateGroup(ctx, groupID, universityID, "PRIVACY", true); err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(24 * time.Hour)
	end := start.AddDate(0, 5, 0)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO semesters (id, university_id, name, start_date, end_date, external_id)
		VALUES ($1,$2,'Privacy semester',$3,$4,$1)`, semesterID, universityID, start, end); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewDataSourceRepository(db).CreateDataSource(
		ctx, sourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewParseLogRepository(db).CreateParseLog(
		ctx, logID, sourceID, "success", 0, "",
	); err != nil {
		t.Fatal(err)
	}

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO admin_sessions
		  (token_hash, admin_id, name, auth_method, admin_role, csrf_token, expires_at)
		  VALUES ($1,$2,'Privacy','telegram','owner','csrf',NOW()+INTERVAL '1 hour')`,
			[]any{"privacy-session-" + suffix, userID}},
		{`INSERT INTO admin_audit_logs
		  (id, actor_id, actor_name, action, object_type, object_id, ip_address)
		  VALUES ($1,$2,'Privacy','update','user',$2,'127.0.0.1')`, []any{auditID, userID}},
		{`INSERT INTO chat_schedule_profiles (chat_id, title, default_group_id, configured_by)
		  VALUES ($1,'Privacy',$2,$3)`, []any{"privacy-chat-" + suffix, groupID, userID}},
		{`INSERT INTO lesson_overrides
		  (id, university_id, semester_id, day_of_week, time_start, time_end,
		   week_type, subject, type, group_id, valid_from, valid_to, created_by)
		  VALUES ($1,$2,$3,1,'08:00','09:35','every','Privacy','lecture',$4,$5,$6,$7)`,
			[]any{overrideID, universityID, semesterID, groupID, start, end, userID}},
		{`INSERT INTO support_requests
		  (id, user_id, request_type, details, status, reviewed_by, reviewed_at)
		  VALUES ($1,$2,'new_institution','Privacy integration request','approved',$3,NOW())`,
			[]any{requestID, otherUserID, userID}},
		{`INSERT INTO parser_snapshots
		  (id, data_source_id, parse_log_id, status, publishable, group_count,
		   lesson_count, payload, reviewed_by, reviewed_at)
		  VALUES ($1,$2,$3,'rejected',FALSE,0,0,'{}'::jsonb,$4,NOW())`,
			[]any{snapshotID, sourceID, logID, userID}},
		{`INSERT INTO connector_clients
		  (id, data_source_id, display_name, key_id, public_key, created_by)
		  VALUES ($1,$2,'Privacy connector',$3,$4,$5)`,
			[]any{connectorID, sourceID, "privacy-key-" + suffix, []byte("integration-public-key"), userID}},
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("create privacy reference: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM connector_clients WHERE id=$1`, connectorID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM support_requests WHERE id=$1`, requestID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM admin_audit_logs WHERE id=$1`, auditID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, otherUserID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID)
	})

	exported, err := users.ExportUserData(ctx, userID)
	if err != nil {
		t.Fatalf("export user data: %v", err)
	}
	categories := make(map[string]bool)
	for _, reference := range exported.References {
		categories[reference.Category] = true
	}
	for _, category := range []string{
		"admin_audit", "admin_session", "chat_schedule_profile", "lesson_override",
		"support_review", "parser_snapshot_review", "connector",
	} {
		if !categories[category] {
			t.Fatalf("export omitted operational reference %q: %+v", category, exported.References)
		}
	}
	if len(exported.AuditRecords) != 1 || exported.AuditRecords[0].IPAddress != "127.0.0.1" ||
		exported.AuditRecords[0].ActorName != "Privacy" {
		t.Fatalf("privacy export omitted audit personal data: %+v", exported.AuditRecords)
	}
	if len(exported.AdminSessions) != 1 || exported.AdminSessions[0].Name != "Privacy" ||
		exported.AdminSessions[0].AuthMethod != "telegram" {
		t.Fatalf("privacy export omitted session metadata: %+v", exported.AdminSessions)
	}

	if err := users.DeleteUser(ctx, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var anonymizedID string
	if err := db.GetContext(ctx, &anonymizedID,
		`SELECT actor_id FROM admin_audit_logs WHERE id=$1`, auditID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(anonymizedID, "deleted:") || strings.Contains(anonymizedID, userID) ||
		len(anonymizedID) != len("deleted:")+32 {
		t.Fatalf("unsafe anonymous marker: %q", anonymizedID)
	}

	var sessions int
	if err := db.GetContext(ctx, &sessions, `SELECT COUNT(*)::int FROM admin_sessions WHERE admin_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("active sessions after deletion=%d", sessions)
	}
	checks := []struct {
		query string
		id    string
		want  string
	}{
		{`SELECT actor_id FROM admin_audit_logs WHERE id=$1`, auditID, anonymizedID},
		{`SELECT configured_by FROM chat_schedule_profiles WHERE chat_id=$1`, "privacy-chat-" + suffix, anonymizedID},
		{`SELECT created_by FROM lesson_overrides WHERE id=$1`, overrideID, anonymizedID},
		{`SELECT reviewed_by FROM support_requests WHERE id=$1`, requestID, anonymizedID},
		{`SELECT reviewed_by FROM parser_snapshots WHERE id=$1`, snapshotID, anonymizedID},
		{`SELECT created_by FROM connector_clients WHERE id=$1`, connectorID, anonymizedID},
	}
	for _, check := range checks {
		var value string
		if err := db.GetContext(ctx, &value, check.query, check.id); err != nil {
			t.Fatal(err)
		}
		if value != check.want {
			t.Fatalf("reference %q=%q, want %q", check.id, value, check.want)
		}
	}
	var audit struct {
		Name string `db:"actor_name"`
		IP   string `db:"ip_address"`
	}
	if err := db.GetContext(ctx, &audit,
		`SELECT actor_name, ip_address FROM admin_audit_logs WHERE id=$1`, auditID); err != nil {
		t.Fatal(err)
	}
	if audit.Name != "Удалённый пользователь" || audit.IP != "" {
		t.Fatalf("audit personal data was not erased: %+v", audit)
	}
}
