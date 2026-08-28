//go:build integration

package repository_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestPublicationPersistsExternalGroupIdentityMapping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openRepositoryIntegrationDB(t, ctx)

	suffix := uuid.NewString()
	universityID := "external-identity-university-" + suffix
	sourceID := "external-identity-source-" + suffix
	canonicalGroupID := "external-identity-canonical-" + suffix
	externalGroupID := "external-identity-source-group-" + suffix
	parseLogID := "external-identity-log-" + suffix
	snapshotID := "external-identity-snapshot-" + suffix
	semesterID := "external-identity-semester-" + suffix
	cleanupRepositoryUniversity(t, db, universityID)

	createRepositoryPublicationFixture(t, ctx, db, universityID, sourceID)
	if _, err := repository.NewGroupRepository(db).CreateGroup(
		ctx, canonicalGroupID, universityID, "1-ЭЭ-В", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewParseLogRepository(db).CreateParseLog(
		ctx, parseLogID, sourceID, "running", 0, "",
	); err != nil {
		t.Fatal(err)
	}

	payload, remapped, err := repository.CanonicalizeSnapshotGroupIDs(
		domain.ScheduleSnapshot{
			UniversityID: universityID,
			SemesterID:   semesterID,
			StartDate:    time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
			Groups: []domain.SnapshotGroup{{
				ID: externalGroupID, UniversityID: universityID, Name: "1-ЭЭ-В",
			}},
		},
		[]domain.Group{{ID: canonicalGroupID, UniversityID: universityID, Name: "1-ЭЭ-В"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 1 || payload.Groups[0].ID != canonicalGroupID || payload.Groups[0].ExternalID != externalGroupID {
		t.Fatalf("unexpected staged identity: %+v", payload.Groups[0])
	}

	snapshots := repository.NewParserSnapshotRepository(db)
	if err = snapshots.Create(ctx, &domain.ParserSnapshot{
		ID: snapshotID, DataSourceID: sourceID, ParseLogID: parseLogID,
		Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1,
		Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshots.Publish(ctx, snapshotID, "integration", "external identity"); err != nil {
		t.Fatal(err)
	}

	var mapping struct {
		GroupID      string `db:"group_id"`
		ExpectedName string `db:"expected_name"`
		PayloadID    string `db:"payload_id"`
	}
	if err = db.GetContext(ctx, &mapping, `
		SELECT mapping.group_id, mapping.expected_name,
		       snapshot.payload #>> '{groups,0,external_id}' AS payload_id
		FROM group_source_identity_mappings mapping
		JOIN parser_snapshots snapshot ON snapshot.id=$3
		WHERE mapping.data_source_id=$1 AND mapping.external_group_id=$2`,
		sourceID, externalGroupID, snapshotID); err != nil {
		t.Fatal(err)
	}
	if mapping.GroupID != canonicalGroupID || mapping.ExpectedName != "1-ЭЭ-В" || mapping.PayloadID != externalGroupID {
		t.Fatalf("unexpected persisted mapping: %+v", mapping)
	}
}

func TestRecordIdentityConflictDoesNotReopenResolvedMapping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openRepositoryIntegrationDB(t, ctx)

	suffix := uuid.NewString()
	universityID := "resolved-identity-university-" + suffix
	sourceID := "resolved-identity-source-" + suffix
	oldGroupID := "resolved-identity-old-" + suffix
	resolvedGroupID := "resolved-identity-new-" + suffix
	externalGroupID := "resolved-identity-external-" + suffix
	conflictID := "resolved-identity-conflict-" + suffix
	cleanupRepositoryUniversity(t, db, universityID)

	createRepositoryPublicationFixture(t, ctx, db, universityID, sourceID)
	groups := repository.NewGroupRepository(db)
	if _, err := groups.CreateGroup(ctx, oldGroupID, universityID, "2-ЭЭ-В", false); err != nil {
		t.Fatal(err)
	}
	if _, err := groups.CreateGroup(ctx, resolvedGroupID, universityID, "1-ЭЭ-В", true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO group_source_identity_mappings (
			data_source_id, external_group_id, group_id, expected_name
		) VALUES ($1,$2,$3,'1-ЭЭ-В')`, sourceID, externalGroupID, resolvedGroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO group_identity_conflicts (
			id, data_source_id, university_id, external_group_id,
			existing_group_id, existing_name, incoming_name, status,
			resolution, resolved_group_id, resolved_by, resolved_at
		) VALUES ($1,$2,$3,$4,$5,'2-ЭЭ-В','1-ЭЭ-В','resolved',
		          'new_group',$6,'integration',NOW())`,
		conflictID, sourceID, universityID, externalGroupID, oldGroupID, resolvedGroupID); err != nil {
		t.Fatal(err)
	}

	if err := groups.RecordIdentityConflict(ctx, sourceID, universityID, &repository.GroupIdentityConflictError{
		ExternalGroupID: externalGroupID,
		ExistingGroupID: oldGroupID,
		ExistingName:    "2-ЭЭ-В",
		IncomingName:    "1-ЭЭ-В",
	}); err != nil {
		t.Fatal(err)
	}
	var state struct {
		Status      string `db:"status"`
		Resolution  string `db:"resolution"`
		Occurrences int    `db:"occurrences"`
	}
	if err := db.GetContext(ctx, &state, `
		SELECT status, resolution, occurrences
		FROM group_identity_conflicts WHERE id=$1`, conflictID); err != nil {
		t.Fatal(err)
	}
	if state.Status != "resolved" || state.Resolution != "new_group" || state.Occurrences != 1 {
		t.Fatalf("resolved conflict was reopened: %+v", state)
	}
}

func TestDisabledSourceCannotPublishSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := openRepositoryIntegrationDB(t, ctx)

	suffix := uuid.NewString()
	universityID := "disabled-publication-university-" + suffix
	sourceID := "disabled-publication-source-" + suffix
	parseLogID := "disabled-publication-log-" + suffix
	snapshotID := "disabled-publication-snapshot-" + suffix
	groupID := "disabled-publication-group-" + suffix
	cleanupRepositoryUniversity(t, db, universityID)

	createRepositoryPublicationFixture(t, ctx, db, universityID, sourceID)
	if _, err := repository.NewParseLogRepository(db).CreateParseLog(
		ctx, parseLogID, sourceID, "running", 0, "",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE data_sources SET is_enabled=FALSE WHERE id=$1`, sourceID,
	); err != nil {
		t.Fatal(err)
	}
	snapshots := repository.NewParserSnapshotRepository(db)
	if err := snapshots.Create(ctx, &domain.ParserSnapshot{
		ID: snapshotID, DataSourceID: sourceID, ParseLogID: parseLogID,
		Status: domain.SnapshotStatusStaged, Publishable: true, GroupCount: 1,
		Payload: domain.ScheduleSnapshot{
			UniversityID: universityID,
			SemesterID:   "disabled-publication-semester-" + suffix,
			StartDate:    time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC),
			EndDate:      time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
			Groups: []domain.SnapshotGroup{{
				ID: groupID, UniversityID: universityID, Name: "DISABLED-" + suffix,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshots.Publish(ctx, snapshotID, "integration", "must fail"); err == nil || !strings.Contains(err.Error(), "is disabled") {
		t.Fatalf("publish disabled source error = %v", err)
	}
	stored, err := snapshots.Get(ctx, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Status != domain.SnapshotStatusStaged {
		t.Fatalf("disabled source changed snapshot state: %+v", stored)
	}
	if _, err = db.ExecContext(ctx,
		`UPDATE data_sources SET is_enabled=TRUE WHERE id=$1`, sourceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshots.Publish(ctx, snapshotID, "integration", "enabled publication"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx,
		`UPDATE data_sources SET is_enabled=FALSE WHERE id=$1`, sourceID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, groupID); err != nil {
		t.Fatal(err)
	}
	if restored, restoreErr := snapshots.RestorePublishedSnapshot(ctx, snapshotID); restoreErr != nil || restored != nil {
		t.Fatalf("restore disabled source: snapshot=%+v err=%v", restored, restoreErr)
	}
	var groupExists bool
	if err = db.GetContext(ctx, &groupExists,
		`SELECT EXISTS (SELECT 1 FROM groups WHERE id=$1)`, groupID,
	); err != nil {
		t.Fatal(err)
	}
	if groupExists {
		t.Fatal("restore republished a snapshot from a disabled source")
	}
}

func createRepositoryPublicationFixture(
	t *testing.T,
	ctx context.Context,
	db *sqlx.DB,
	universityID string,
	sourceID string,
) {
	t.Helper()
	if _, err := repository.NewUniversityRepository(db).CreateUniversity(
		ctx, universityID, "Integration university", "Integration university", "https://example.test", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewDataSourceRepository(db).CreateDataSource(
		ctx, sourceID, universityID, "integration", "{}", 3600,
	); err != nil {
		t.Fatal(err)
	}
}

func cleanupRepositoryUniversity(t *testing.T, db *sqlx.DB, universityID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, `DELETE FROM universities WHERE id=$1`, universityID); err != nil {
			t.Errorf("cleanup university %s: %v", universityID, err)
		}
	})
}

func openRepositoryIntegrationDB(t *testing.T, ctx context.Context) *sqlx.DB {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close integration database: %v", err)
		}
	})
	if err := database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	return db
}
