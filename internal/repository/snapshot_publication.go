package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

func (r *ParserSnapshotRepository) Approve(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
) (*domain.ParserSnapshot, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("approve snapshot: begin: %w", err)
	}
	defer tx.Rollback()

	row, err := loadPublicationSnapshot(ctx, tx, snapshotID, true)
	if err != nil {
		return nil, err
	}
	if !row.Publishable {
		return nil, fmt.Errorf("snapshot %s is structurally invalid", snapshotID)
	}
	if row.Status == domain.SnapshotStatusRejected {
		return nil, fmt.Errorf("snapshot %s was rejected", snapshotID)
	}
	var lifecycle string
	if err = tx.GetContext(ctx, &lifecycle,
		`SELECT lifecycle_status FROM data_sources WHERE id=$1 FOR UPDATE`, row.DataSourceID,
	); err != nil {
		return nil, fmt.Errorf("approve snapshot: load source: %w", err)
	}
	if lifecycle == domain.ConnectorStatusActive {
		return nil, fmt.Errorf("snapshot %s belongs to an active source and must be published", snapshotID)
	}
	now := time.Now()
	if _, err = tx.ExecContext(ctx, `
		UPDATE parser_snapshots
		SET status='approved', reviewed_by=$2, review_note=$3, reviewed_at=$4
		WHERE id=$1`, snapshotID, actorID, reviewNote, now); err != nil {
		return nil, fmt.Errorf("approve snapshot: update: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("approve snapshot: commit: %w", err)
	}
	snapshot, err := decodeParserSnapshot(*row)
	if err != nil {
		return nil, err
	}
	snapshot.Status = domain.SnapshotStatusApproved
	snapshot.ReviewedAt = &now
	snapshot.ReviewedBy = actorID
	snapshot.ReviewNote = reviewNote
	return snapshot, nil
}

func (r *ParserSnapshotRepository) PublishWithHook(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
	hook SnapshotPublicationHook,
) (*domain.ParserSnapshot, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: begin: %w", err)
	}
	defer tx.Rollback()

	row, err := loadPublicationSnapshot(ctx, tx, snapshotID, false)
	if err != nil {
		return nil, err
	}
	var universityID string
	if err = tx.GetContext(ctx, &universityID,
		`SELECT university_id FROM data_sources WHERE id=$1`, row.DataSourceID,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: load source university: %w", err)
	}
	if err = lockUniversityPublication(ctx, tx, universityID); err != nil {
		return nil, err
	}
	row, err = loadPublicationSnapshot(ctx, tx, snapshotID, true)
	if err != nil {
		return nil, err
	}
	var source publicationSource
	if err = tx.GetContext(ctx, &source, `
		SELECT adapter_type, lifecycle_status, university_id
		FROM data_sources WHERE id=$1 FOR UPDATE`, row.DataSourceID); err != nil {
		return nil, fmt.Errorf("publish snapshot: lock source: %w", err)
	}
	if source.Lifecycle != domain.ConnectorStatusActive {
		return nil, fmt.Errorf("publish snapshot: source %s is not active", row.DataSourceID)
	}
	snapshot, err := applyPublicationSnapshot(
		ctx, tx, row, source.AdapterType, source.UniversityID, actorID, reviewNote, hook, true,
	)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("publish snapshot: commit: %w", err)
	}
	return snapshot, nil
}

func (r *ParserSnapshotRepository) RestorePublishedSnapshot(
	ctx context.Context,
	snapshotID string,
) (*domain.ParserSnapshot, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("restore published snapshot: begin: %w", err)
	}
	defer tx.Rollback()

	row, err := loadPublicationSnapshot(ctx, tx, snapshotID, false)
	if err != nil {
		return nil, err
	}
	var universityID string
	if err = tx.GetContext(ctx, &universityID,
		`SELECT university_id FROM data_sources WHERE id=$1`, row.DataSourceID,
	); err != nil {
		return nil, fmt.Errorf("restore published snapshot: load source university: %w", err)
	}
	if err = lockUniversityPublication(ctx, tx, universityID); err != nil {
		return nil, err
	}
	row, err = loadPublicationSnapshot(ctx, tx, snapshotID, true)
	if err != nil {
		return nil, err
	}
	if row.Status != domain.SnapshotStatusPublished || row.PublishedAt == nil {
		return nil, fmt.Errorf("restore published snapshot: snapshot %s is not published", snapshotID)
	}
	type restorationSource struct {
		AdapterType        string         `db:"adapter_type"`
		Lifecycle          string         `db:"lifecycle_status"`
		UniversityID       string         `db:"university_id"`
		CurrentSnapshotID  string         `db:"current_snapshot_id"`
		LastSuccessAt      *time.Time     `db:"last_success_at"`
		LastRunAt          *time.Time     `db:"last_run_at"`
		LastError          sql.NullString `db:"last_error"`
		ConsecutiveFailure int            `db:"consecutive_failures"`
		NextRetryAt        *time.Time     `db:"next_retry_at"`
		UpdatedAt          time.Time      `db:"updated_at"`
	}
	var source restorationSource
	if err = tx.GetContext(ctx, &source, `
		SELECT adapter_type, lifecycle_status, university_id,
		       COALESCE(current_snapshot_id, '') AS current_snapshot_id,
		       last_success_at, last_run_at, last_error,
		       consecutive_failures, next_retry_at, updated_at
		FROM data_sources WHERE id=$1 FOR UPDATE`, row.DataSourceID); err != nil {
		return nil, fmt.Errorf("restore published snapshot: lock source: %w", err)
	}
	if source.Lifecycle != domain.ConnectorStatusActive || source.CurrentSnapshotID != snapshotID {
		var newerPublicationExists bool
		if err = tx.GetContext(ctx, &newerPublicationExists, `
			SELECT EXISTS (
				SELECT 1
				FROM data_sources current_source
				JOIN parser_snapshots current_snapshot
				  ON current_snapshot.id=current_source.current_snapshot_id
				WHERE current_source.university_id=$1
				  AND current_source.lifecycle_status='active'
				  AND current_snapshot.status='published'
			)`, source.UniversityID); err != nil {
			return nil, fmt.Errorf("restore published snapshot: inspect newer publication: %w", err)
		}
		if !newerPublicationExists {
			return nil, fmt.Errorf(
				"restore published snapshot: source %s is no longer the active current publication",
				row.DataSourceID,
			)
		}
		return nil, tx.Commit()
	}
	originalPublishedAt := *row.PublishedAt
	snapshot, err := applyPublicationSnapshot(
		ctx, tx, row, source.AdapterType, source.UniversityID, "", "", nil, false,
	)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE parser_snapshots SET published_at=$2 WHERE id=$1`,
		snapshotID, originalPublishedAt,
	); err != nil {
		return nil, fmt.Errorf("restore published snapshot: preserve publication time: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources
		SET last_success_at=$2, last_run_at=$3, last_error=$4,
		    consecutive_failures=$5, next_retry_at=$6, updated_at=$7
		WHERE id=$1`, row.DataSourceID, source.LastSuccessAt, source.LastRunAt,
		source.LastError, source.ConsecutiveFailure, source.NextRetryAt, source.UpdatedAt); err != nil {
		return nil, fmt.Errorf("restore published snapshot: preserve source history: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("restore published snapshot: commit: %w", err)
	}
	snapshot.PublishedAt = &originalPublishedAt
	return snapshot, nil
}

func (r *ParserSnapshotRepository) ActivateConnectorWithSnapshot(
	ctx context.Context,
	connectorID, snapshotID, actorID, reviewNote string,
	hook SnapshotPublicationHook,
) (*domain.ParserSnapshot, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("activate connector: begin: %w", err)
	}
	defer tx.Rollback()

	var target connectorPublicationTarget
	if err = tx.GetContext(ctx, &target, `
		SELECT c.data_source_id, ds.university_id, ds.adapter_type
		FROM connector_clients c
		JOIN data_sources ds ON ds.id=c.data_source_id
		WHERE c.id=$1`, connectorID); errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("activate connector: load target: %w", err)
	}
	if err = lockUniversityPublication(ctx, tx, target.UniversityID); err != nil {
		return nil, err
	}
	if err = tx.GetContext(ctx, &target, `
		SELECT c.data_source_id, ds.university_id, ds.adapter_type
		FROM connector_clients c
		JOIN data_sources ds ON ds.id=c.data_source_id
		WHERE c.id=$1
		FOR UPDATE OF c, ds`, connectorID); err != nil {
		return nil, fmt.Errorf("activate connector: lock target: %w", err)
	}
	var sourceIDs []string
	if err = tx.SelectContext(ctx, &sourceIDs, `
		SELECT id FROM data_sources
		WHERE university_id=$1
		ORDER BY id FOR UPDATE`, target.UniversityID); err != nil {
		return nil, fmt.Errorf("activate connector: lock university sources: %w", err)
	}
	row, err := loadPublicationSnapshot(ctx, tx, snapshotID, true)
	if err != nil {
		return nil, err
	}
	if row.DataSourceID != target.SourceID {
		return nil, fmt.Errorf("snapshot %s does not belong to connector %s", snapshotID, connectorID)
	}
	if row.Status != domain.SnapshotStatusApproved {
		return nil, fmt.Errorf("snapshot %s must be approved before activation", snapshotID)
	}
	if !row.Publishable {
		return nil, fmt.Errorf("snapshot %s is structurally invalid", snapshotID)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE connector_clients c
		SET status='suspended', updated_at=NOW()
		FROM data_sources ds
		WHERE c.data_source_id=ds.id
		  AND ds.university_id=$1
		  AND ds.id<>$2
		  AND c.status='active'`, target.UniversityID, target.SourceID); err != nil {
		return nil, fmt.Errorf("activate connector: suspend previous connectors: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources
		SET lifecycle_status='suspended', is_enabled=FALSE, updated_at=NOW()
		WHERE university_id=$1 AND id<>$2 AND lifecycle_status='active'`,
		target.UniversityID, target.SourceID); err != nil {
		return nil, fmt.Errorf("activate connector: suspend previous source: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE connector_clients SET status='active', updated_at=NOW() WHERE id=$1`, connectorID,
	); err != nil {
		return nil, fmt.Errorf("activate connector: enable connector: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources
		SET lifecycle_status='active', is_enabled=TRUE, archived_at=NULL, updated_at=NOW()
		WHERE id=$1`, target.SourceID); err != nil {
		return nil, fmt.Errorf("activate connector: enable source: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE universities SET is_active=TRUE, updated_at=NOW() WHERE id=$1`, target.UniversityID,
	); err != nil {
		return nil, fmt.Errorf("activate connector: enable university: %w", err)
	}
	snapshot, err := applyPublicationSnapshot(
		ctx, tx, row, target.AdapterType, target.UniversityID, actorID, reviewNote, hook, false,
	)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		WITH updated AS (
			UPDATE connector_ingestion_runs
			SET status='published', group_count=$3, lesson_count=$4,
				error_message='', completed_at=NOW(), claimed_at=NULL
			WHERE connector_id=$1 AND parser_snapshot_id=$2
			RETURNING connector_id
		)
		UPDATE connector_clients c SET last_snapshot_at=NOW(), updated_at=NOW()
		FROM updated WHERE c.id=updated.connector_id`,
		connectorID, snapshotID, snapshot.GroupCount, snapshot.LessonCount); err != nil {
		return nil, fmt.Errorf("activate connector: complete ingestion: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("activate connector: commit: %w", err)
	}
	return snapshot, nil
}

type publicationSource struct {
	AdapterType  string `db:"adapter_type"`
	Lifecycle    string `db:"lifecycle_status"`
	UniversityID string `db:"university_id"`
}

type connectorPublicationTarget struct {
	SourceID     string `db:"data_source_id"`
	UniversityID string `db:"university_id"`
	AdapterType  string `db:"adapter_type"`
}

func lockUniversityPublication(ctx context.Context, tx *sqlx.Tx, universityID string) error {
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('scheduler-snapshot-publication'), hashtext($1))`,
		universityID,
	); err != nil {
		return fmt.Errorf("acquire university publication lock: %w", err)
	}
	return nil
}

func loadPublicationSnapshot(
	ctx context.Context,
	tx *sqlx.Tx,
	snapshotID string,
	forUpdate bool,
) (*parserSnapshotRow, error) {
	query := `SELECT ` + parserSnapshotColumns + ` FROM parser_snapshots WHERE id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var row parserSnapshotRow
	if err := tx.GetContext(ctx, &row, query, snapshotID); errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("load parser snapshot %s: %w", snapshotID, err)
	}
	return &row, nil
}

func applyPublicationSnapshot(
	ctx context.Context,
	tx *sqlx.Tx,
	row *parserSnapshotRow,
	adapterType, universityID, actorID, reviewNote string,
	hook SnapshotPublicationHook,
	finalizeParseLog bool,
) (*domain.ParserSnapshot, error) {
	if !row.Publishable {
		return nil, fmt.Errorf("snapshot %s is structurally invalid", row.ID)
	}
	if row.Status == domain.SnapshotStatusRejected {
		return nil, fmt.Errorf("snapshot %s was rejected", row.ID)
	}
	snapshot, err := decodeParserSnapshot(*row)
	if err != nil {
		return nil, err
	}
	payload := snapshot.Payload
	if payload.UniversityID != universityID {
		return nil, fmt.Errorf(
			"snapshot %s belongs to university %s, expected %s",
			row.ID, payload.UniversityID, universityID,
		)
	}
	if adapterType == domain.IntegrationModeExternalPush {
		payload = normalizeExternalParityRecurrence(payload)
	}
	var existingGroups []domain.Group
	if err = tx.SelectContext(ctx, &existingGroups, `
		SELECT id, university_id, name, is_active, created_at, updated_at
		FROM groups WHERE university_id=$1`, universityID); err != nil {
		return nil, fmt.Errorf("publish snapshot: load group identities: %w", err)
	}
	payload, _, err = CanonicalizeSnapshotGroupIDs(payload, existingGroups)
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: reconcile group identities: %w", err)
	}
	payload, err = reconcileLessonIdentities(ctx, tx, payload, row.DataSourceID)
	if err != nil {
		return nil, err
	}
	snapshot.Payload = payload
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: encode canonical payload: %w", err)
	}
	if err = applyPublicationMetadata(ctx, tx, payload); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE groups SET is_active=FALSE, updated_at=NOW()
		WHERE university_id=$1 AND is_active`, universityID); err != nil {
		return nil, fmt.Errorf("publish snapshot: deactivate groups: %w", err)
	}
	for _, group := range payload.Groups {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO groups (id, university_id, name, is_active, created_at, updated_at)
			VALUES ($1,$2,$3,TRUE,NOW(),NOW())
			ON CONFLICT (id) DO UPDATE SET
				university_id=EXCLUDED.university_id, name=EXCLUDED.name,
				is_active=TRUE, updated_at=NOW()`,
			group.ID, group.UniversityID, group.Name); err != nil {
			return nil, fmt.Errorf("publish snapshot: group %s: %w", group.ID, err)
		}
	}
	if err = storeLessonIdentities(ctx, tx, payload); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM lessons WHERE university_id=$1`, universityID,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: clear lessons: %w", err)
	}
	if err = insertPublicationLessons(ctx, tx, payload); err != nil {
		return nil, err
	}
	now := time.Now()
	if _, err = tx.ExecContext(ctx, `
		UPDATE parser_snapshots
		SET status='published', reviewed_by=CASE WHEN $2='' THEN reviewed_by ELSE $2 END,
			review_note=CASE WHEN $2='' THEN review_note ELSE $3 END,
			reviewed_at=CASE WHEN $2='' THEN reviewed_at ELSE $4 END,
			published_at=$4, payload=$5::jsonb
		WHERE id=$1`, row.ID, actorID, reviewNote, now, canonicalPayload); err != nil {
		return nil, fmt.Errorf("publish snapshot: mark published: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources
		SET current_snapshot_id=$2, last_success_at=$3, last_run_at=$3,
			last_error='', consecutive_failures=0, next_retry_at=NULL, updated_at=$3
		WHERE id=$1`, row.DataSourceID, row.ID, now); err != nil {
		return nil, fmt.Errorf("publish snapshot: update source: %w", err)
	}
	if finalizeParseLog && row.ParseLogID != "" {
		result, finalizeErr := tx.ExecContext(ctx, `
			UPDATE parse_logs
			SET finished_at=NOW(), status='success', records_fetched=$2,
				error_message=NULL
			WHERE id=$1 AND status='running'`, row.ParseLogID, row.LessonCount)
		if finalizeErr != nil {
			return nil, fmt.Errorf("publish snapshot: finalize parse log: %w", finalizeErr)
		}
		if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
			if countErr != nil {
				return nil, fmt.Errorf("publish snapshot: count finalized parse log: %w", countErr)
			}
			var existingStatus string
			if statusErr := tx.GetContext(ctx, &existingStatus,
				`SELECT status FROM parse_logs WHERE id=$1`, row.ParseLogID); statusErr != nil {
				return nil, fmt.Errorf("publish snapshot: inspect parse log finalization: %w", statusErr)
			}
			if existingStatus != "success" && existingStatus != "quarantined" {
				return nil, fmt.Errorf("publish snapshot: parse log %s has invalid status %s", row.ParseLogID, existingStatus)
			}
		}
	}
	if hook != nil {
		if err = hook(ctx, &SnapshotPublication{tx: tx}); err != nil {
			return nil, fmt.Errorf("publish snapshot: transactional hook: %w", err)
		}
	}
	snapshot.Status = domain.SnapshotStatusPublished
	snapshot.PublishedAt = &now
	if actorID != "" {
		snapshot.ReviewedAt = &now
		snapshot.ReviewedBy = actorID
		snapshot.ReviewNote = reviewNote
	}
	return snapshot, nil
}

const publicationLessonBatchSize = 500

func insertPublicationLessons(
	ctx context.Context,
	tx *sqlx.Tx,
	payload domain.ScheduleSnapshot,
) error {
	type groupLesson struct {
		groupID string
		lesson  domain.Lesson
	}
	lessons := make([]groupLesson, 0)
	for _, group := range payload.Groups {
		for _, lesson := range group.Lessons {
			lessons = append(lessons, groupLesson{groupID: group.ID, lesson: lesson})
		}
	}
	for start := 0; start < len(lessons); start += publicationLessonBatchSize {
		end := min(start+publicationLessonBatchSize, len(lessons))
		var query strings.Builder
		query.WriteString(`INSERT INTO lessons (
			id, university_id, semester_id, day_of_week, special_date,
			time_start, time_end, week_type, subject, type, teacher, room,
			group_id, subgroup, valid_from, valid_to, recurrence, source_id,
			external_id, fetched_at, source_fingerprint, updated_at
		) VALUES `)
		args := make([]any, 0, (end-start)*21)
		for index, item := range lessons[start:end] {
			if index > 0 {
				query.WriteByte(',')
			}
			base := index * 21
			query.WriteByte('(')
			for column := 1; column <= 21; column++ {
				if column > 1 {
					query.WriteByte(',')
				}
				fmt.Fprintf(&query, "$%d", base+column)
			}
			query.WriteString(",NOW())")
			lesson := item.lesson
			args = append(args,
				lesson.ID, lesson.UniversityID, payload.SemesterID,
				lessonDayOfWeekDB(lesson), lesson.SpecialDate,
				lesson.TimeStart, lesson.TimeEnd, lesson.WeekType,
				lesson.Subject, lesson.Type, lesson.Teacher, lesson.Room,
				item.groupID, lesson.Subgroup, lesson.ValidFrom, lesson.ValidTo,
				lesson.Recurrence, nullIfEmpty(lesson.SourceID), lesson.ExternalID,
				lesson.FetchedAt, lesson.SourceFingerprint,
			)
		}
		if _, err := tx.ExecContext(ctx, query.String(), args...); err != nil {
			return fmt.Errorf("publish snapshot: insert lesson batch %d-%d: %w", start, end, err)
		}
	}
	return nil
}

func applyPublicationMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	payload domain.ScheduleSnapshot,
) error {
	metadata := payload.Metadata
	if metadata != nil {
		institution := metadata.Institution
		if _, err := tx.ExecContext(ctx, `
			UPDATE universities SET
				name=CASE WHEN $2='' THEN name ELSE $2 END,
				full_name=CASE WHEN $3='' THEN full_name ELSE $3 END,
				schedule_url=CASE WHEN $4='' THEN schedule_url ELSE $4 END,
				timezone=CASE WHEN $5='' THEN timezone ELSE $5 END,
				locale=CASE WHEN $6='' THEN locale ELSE $6 END,
				updated_at=NOW()
			WHERE id=$1`, payload.UniversityID, institution.Name, institution.FullName,
			institution.ScheduleURL, institution.Timezone, institution.Locale); err != nil {
			return fmt.Errorf("publish snapshot: update university metadata: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE semesters SET status='archived', updated_at=NOW()
			WHERE university_id=$1 AND status='active' AND id<>$2`,
			payload.UniversityID, payload.SemesterID); err != nil {
			return fmt.Errorf("publish snapshot: archive previous terms: %w", err)
		}
		term := metadata.Term
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO semesters (
				id, university_id, external_id, name, academic_year,
				status, start_date, end_date
			) VALUES ($1,$2,$3,$4,$5,'active',$6,$7)
			ON CONFLICT (id) DO UPDATE SET
				university_id=EXCLUDED.university_id, external_id=EXCLUDED.external_id,
				name=EXCLUDED.name, academic_year=EXCLUDED.academic_year,
				status='active', start_date=EXCLUDED.start_date,
				end_date=EXCLUDED.end_date, updated_at=NOW()`,
			payload.SemesterID, payload.UniversityID, term.ExternalID, term.Name,
			term.AcademicYear, payload.StartDate, payload.EndDate); err != nil {
			return fmt.Errorf("publish snapshot: upsert connector term: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO semesters (id, university_id, external_id, name, start_date, end_date)
		VALUES ($1,$2,$1,'Актуальный снимок',$3,$4)
		ON CONFLICT (id) DO UPDATE SET
			university_id=EXCLUDED.university_id, start_date=EXCLUDED.start_date,
			end_date=EXCLUDED.end_date, updated_at=NOW()`,
		payload.SemesterID, payload.UniversityID, payload.StartDate, payload.EndDate); err != nil {
		return fmt.Errorf("publish snapshot: semester: %w", err)
	}
	return nil
}
