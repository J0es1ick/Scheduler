package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type ParserSnapshotRepository struct {
	db *sqlx.DB
}

type SnapshotPublication struct {
	tx *sqlx.Tx
}

type SnapshotPublicationHook func(context.Context, *SnapshotPublication) error

func NewParserSnapshotRepository(db *sqlx.DB) *ParserSnapshotRepository {
	return &ParserSnapshotRepository{db: db}
}

type parserSnapshotRow struct {
	ID           string     `db:"id"`
	DataSourceID string     `db:"data_source_id"`
	ParseLogID   string     `db:"parse_log_id"`
	Status       string     `db:"status"`
	Publishable  bool       `db:"publishable"`
	GroupCount   int        `db:"group_count"`
	LessonCount  int        `db:"lesson_count"`
	ReasonsRaw   []byte     `db:"anomaly_reasons"`
	PayloadRaw   []byte     `db:"payload"`
	ReviewedBy   string     `db:"reviewed_by"`
	ReviewNote   string     `db:"review_note"`
	CreatedAt    time.Time  `db:"created_at"`
	PublishedAt  *time.Time `db:"published_at"`
	ReviewedAt   *time.Time `db:"reviewed_at"`
}

const parserSnapshotColumns = `
	id, data_source_id, parse_log_id, status, publishable,
	group_count, lesson_count, anomaly_reasons, payload,
	reviewed_by, review_note, created_at, published_at, reviewed_at`

func (r *ParserSnapshotRepository) Create(ctx context.Context, snapshot *domain.ParserSnapshot) error {
	reasons, err := json.Marshal(snapshot.AnomalyReasons)
	if err != nil {
		return fmt.Errorf("marshal snapshot anomalies: %w", err)
	}
	payload, err := json.Marshal(snapshot.Payload)
	if err != nil {
		return fmt.Errorf("marshal parser snapshot: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO parser_snapshots (
			id, data_source_id, parse_log_id, status, publishable,
			group_count, lesson_count, anomaly_reasons, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb)`,
		snapshot.ID, snapshot.DataSourceID, snapshot.ParseLogID,
		snapshot.Status, snapshot.Publishable, snapshot.GroupCount,
		snapshot.LessonCount, reasons, payload,
	)
	if err != nil {
		return fmt.Errorf("create parser snapshot %s: %w", snapshot.ID, err)
	}
	return nil
}

func (r *ParserSnapshotRepository) Get(ctx context.Context, id string) (*domain.ParserSnapshot, error) {
	var row parserSnapshotRow
	err := r.db.GetContext(ctx, &row,
		`SELECT `+parserSnapshotColumns+` FROM parser_snapshots WHERE id=$1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get parser snapshot %s: %w", id, err)
	}
	return decodeParserSnapshot(row)
}

func (r *ParserSnapshotRepository) List(
	ctx context.Context,
	sourceID, status string,
	limit int,
) ([]domain.ParserSnapshot, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	where := "TRUE"
	args := []any{}
	if sourceID != "" {
		args = append(args, sourceID)
		where += fmt.Sprintf(" AND data_source_id=$%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}
	args = append(args, limit)
	var rows []parserSnapshotRow
	query := fmt.Sprintf(`
		SELECT %s FROM parser_snapshots
		WHERE %s ORDER BY created_at DESC LIMIT $%d`,
		parserSnapshotColumns, where, len(args))
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list parser snapshots: %w", err)
	}
	items := make([]domain.ParserSnapshot, 0, len(rows))
	for _, row := range rows {
		item, err := decodeParserSnapshot(row)
		if err != nil {
			return nil, err
		}
		item.Payload = domain.ScheduleSnapshot{}
		items = append(items, *item)
	}
	return items, nil
}

func (r *ParserSnapshotRepository) Baseline(
	ctx context.Context,
	universityID, sourceID string,
) (*domain.SnapshotBaseline, error) {
	result := &domain.SnapshotBaseline{LessonsByGroup: make(map[string]int)}
	if err := r.db.GetContext(
		ctx,
		&result.CurrentSnapshot,
		`SELECT COALESCE(current_snapshot_id, '') FROM data_sources WHERE id=$1`,
		sourceID,
	); err != nil {
		return nil, fmt.Errorf("load snapshot baseline: %w", err)
	}

	if result.CurrentSnapshot != "" {
		var payloadRaw []byte
		if err := r.db.GetContext(ctx, &payloadRaw, `
			SELECT payload
			FROM parser_snapshots
			WHERE id=$1 AND data_source_id=$2 AND status='published'`,
			result.CurrentSnapshot, sourceID,
		); err != nil {
			return nil, fmt.Errorf("load trusted snapshot %s: %w", result.CurrentSnapshot, err)
		}
		var payload domain.ScheduleSnapshot
		if err := json.Unmarshal(payloadRaw, &payload); err != nil {
			return nil, fmt.Errorf("decode trusted snapshot %s: %w", result.CurrentSnapshot, err)
		}
		if payload.UniversityID != universityID {
			return nil, fmt.Errorf(
				"trusted snapshot %s belongs to university %s, expected %s",
				result.CurrentSnapshot,
				payload.UniversityID,
				universityID,
			)
		}
		result.TrustedSnapshot = &payload
		result.GroupCount = len(payload.Groups)
		for _, group := range payload.Groups {
			result.LessonsByGroup[group.ID] = len(group.Lessons)
			result.LessonCount += len(group.Lessons)
		}
		result.HasExistingState = true
		return result, nil
	}

	if err := r.db.GetContext(ctx, result, `
		SELECT
			(SELECT COUNT(*)::int FROM groups
			 WHERE university_id=$1 AND is_active) AS group_count,
			(SELECT COUNT(*)::int FROM lessons
			 WHERE university_id=$1) AS lesson_count,
			'' AS current_snapshot`,
		universityID,
	); err != nil {
		return nil, fmt.Errorf("load legacy schedule baseline: %w", err)
	}
	var counts []struct {
		GroupID string `db:"group_id"`
		Count   int    `db:"count"`
	}
	if err := r.db.SelectContext(ctx, &counts, `
		SELECT g.id AS group_id, COUNT(l.id)::int AS count
		FROM groups g
		LEFT JOIN lessons l ON l.group_id=g.id
		WHERE g.university_id=$1 AND g.is_active
		GROUP BY g.id`, universityID); err != nil {
		return nil, fmt.Errorf("load snapshot group counts: %w", err)
	}
	for _, item := range counts {
		result.LessonsByGroup[item.GroupID] = item.Count
	}
	result.HasExistingState = result.GroupCount > 0 || result.LessonCount > 0
	return result, nil
}

func (r *ParserSnapshotRepository) Publish(
	ctx context.Context,
	snapshotID, actorID, reviewNote string,
) (*domain.ParserSnapshot, error) {
	return r.PublishWithHook(ctx, snapshotID, actorID, reviewNote, nil)
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

	var row parserSnapshotRow
	err = tx.GetContext(ctx, &row,
		`SELECT `+parserSnapshotColumns+` FROM parser_snapshots WHERE id=$1 FOR UPDATE`,
		snapshotID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: load: %w", err)
	}
	if !row.Publishable {
		return nil, fmt.Errorf("snapshot %s is structurally invalid", snapshotID)
	}
	if row.Status == domain.SnapshotStatusRejected {
		return nil, fmt.Errorf("snapshot %s was rejected", snapshotID)
	}
	snapshot, err := decodeParserSnapshot(row)
	if err != nil {
		return nil, err
	}
	payload := snapshot.Payload
	var adapterType string
	if err = tx.GetContext(ctx, &adapterType,
		`SELECT adapter_type FROM data_sources WHERE id=$1`, snapshot.DataSourceID,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: load source type: %w", err)
	}
	if adapterType == domain.IntegrationModeExternalPush {
		payload = normalizeExternalParityRecurrence(payload)
	}
	if _, err = tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('scheduler-snapshot-publication'), hashtext($1))`,
		payload.UniversityID,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: acquire university lock: %w", err)
	}
	var existingGroups []domain.Group
	if err = tx.SelectContext(ctx, &existingGroups, `
		SELECT id, university_id, name, is_active, created_at, updated_at
		FROM groups
		WHERE university_id=$1`, payload.UniversityID); err != nil {
		return nil, fmt.Errorf("publish snapshot: load group identities: %w", err)
	}
	payload, _, err = CanonicalizeSnapshotGroupIDs(payload, existingGroups)
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: reconcile group identities: %w", err)
	}
	snapshot.Payload = payload
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("publish snapshot: encode canonical payload: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO semesters (id, university_id, external_id, name, start_date, end_date)
		VALUES ($1,$2,$1,'Актуальный снимок',$3,$4)
		ON CONFLICT (id) DO UPDATE SET
			university_id=EXCLUDED.university_id,
			start_date=EXCLUDED.start_date,
			end_date=EXCLUDED.end_date,
			updated_at=NOW()`,
		payload.SemesterID, payload.UniversityID, payload.StartDate, payload.EndDate,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: semester: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE groups SET is_active=FALSE, updated_at=NOW()
		WHERE university_id=$1 AND is_active`, payload.UniversityID); err != nil {
		return nil, fmt.Errorf("publish snapshot: deactivate groups: %w", err)
	}
	for _, group := range payload.Groups {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO groups (id, university_id, name, is_active, created_at, updated_at)
			VALUES ($1,$2,$3,TRUE,NOW(),NOW())
			ON CONFLICT (id) DO UPDATE SET
				university_id=EXCLUDED.university_id,
				name=EXCLUDED.name,
				is_active=TRUE,
				updated_at=NOW()`,
			group.ID, group.UniversityID, group.Name,
		); err != nil {
			return nil, fmt.Errorf("publish snapshot: group %s: %w", group.ID, err)
		}
	}
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM lessons WHERE university_id=$1 AND semester_id=$2`, payload.UniversityID, payload.SemesterID,
	); err != nil {
		return nil, fmt.Errorf("publish snapshot: clear lessons: %w", err)
	}
	const insertLesson = `
		INSERT INTO lessons (
			id, university_id, semester_id, day_of_week, special_date,
			time_start, time_end, week_type, subject, type, teacher, room,
			group_id, subgroup, valid_from, valid_to, recurrence, source_id,
			external_id, fetched_at, source_fingerprint, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,NOW())`
	for _, group := range payload.Groups {
		for _, lesson := range group.Lessons {
			if _, err = tx.ExecContext(ctx, insertLesson,
				lesson.ID, lesson.UniversityID, payload.SemesterID,
				lessonDayOfWeekDB(lesson), lesson.SpecialDate,
				lesson.TimeStart, lesson.TimeEnd, lesson.WeekType,
				lesson.Subject, lesson.Type, lesson.Teacher, lesson.Room,
				group.ID, lesson.Subgroup, lesson.ValidFrom, lesson.ValidTo,
				lesson.Recurrence, nullIfEmpty(lesson.SourceID), lesson.ExternalID,
				lesson.FetchedAt, lesson.SourceFingerprint,
			); err != nil {
				return nil, fmt.Errorf("publish snapshot: lesson %s: %w", lesson.ID, err)
			}
		}
	}
	now := time.Now()
	if _, err = tx.ExecContext(ctx, `
		UPDATE parser_snapshots
		SET status='published', reviewed_by=$2, review_note=$3,
			reviewed_at=CASE WHEN $2='' THEN reviewed_at ELSE $4 END,
			published_at=$4, payload=$5::jsonb
		WHERE id=$1`, snapshotID, actorID, reviewNote, now, canonicalPayload); err != nil {
		return nil, fmt.Errorf("publish snapshot: mark published: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE data_sources
		SET current_snapshot_id=$2, last_success_at=$3, last_run_at=$3,
			last_error='', consecutive_failures=0, next_retry_at=NULL,
			updated_at=$3
		WHERE id=$1`, snapshot.DataSourceID, snapshotID, now); err != nil {
		return nil, fmt.Errorf("publish snapshot: update source: %w", err)
	}
	if hook != nil {
		if err = hook(ctx, &SnapshotPublication{tx: tx}); err != nil {
			return nil, fmt.Errorf("publish snapshot: transactional hook: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("publish snapshot: commit: %w", err)
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

func normalizeExternalParityRecurrence(payload domain.ScheduleSnapshot) domain.ScheduleSnapshot {
	for groupIndex := range payload.Groups {
		for lessonIndex := range payload.Groups[groupIndex].Lessons {
			lesson := &payload.Groups[groupIndex].Lessons[lessonIndex]
			if !lesson.Recurrence.IsZero() ||
				(lesson.WeekType != domain.WeekTypeOdd && lesson.WeekType != domain.WeekTypeEven) {
				continue
			}
			cycleWeek := 1
			if lesson.WeekType == domain.WeekTypeEven {
				cycleWeek = 2
			}
			anchor := payload.StartDate
			lesson.Recurrence = domain.RecurrenceRule{
				CycleLength: 2,
				CycleWeeks:  []int{cycleWeek},
				AnchorDate:  &anchor,
			}
		}
	}
	return payload
}

func (p *SnapshotPublication) EffectiveLessonsByUniversity(
	ctx context.Context,
	universityID string,
) ([]domain.Lesson, error) {
	var lessons []domain.Lesson
	if err := p.tx.SelectContext(
		ctx,
		&lessons,
		lessonSelect+` WHERE university_id=$1 ORDER BY group_id, day_of_week, time_start`,
		universityID,
	); err != nil {
		return nil, fmt.Errorf("publication read effective lessons for university %s: %w", universityID, err)
	}
	return lessons, nil
}

func (p *SnapshotPublication) EnqueueScheduleChange(
	ctx context.Context,
	eventID, groupID, source, summary string,
) error {
	if _, err := p.tx.ExecContext(
		ctx,
		`SELECT enqueue_schedule_change($1, $2, $3, $4)`,
		eventID, groupID, source, summary,
	); err != nil {
		return fmt.Errorf("publication enqueue schedule change for group %s: %w", groupID, err)
	}
	return nil
}

func (r *ParserSnapshotRepository) Reject(
	ctx context.Context,
	id, actorID, note string,
) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE parser_snapshots
		SET status='rejected', reviewed_by=$2, review_note=$3, reviewed_at=NOW()
		WHERE id=$1 AND status='quarantined'`, id, actorID, note)
	if err != nil {
		return fmt.Errorf("reject parser snapshot %s: %w", id, err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return sql.ErrNoRows
	}
	_, _ = r.db.ExecContext(ctx, `
		UPDATE data_sources ds
		SET last_error='', updated_at=NOW()
		WHERE ds.id=(SELECT data_source_id FROM parser_snapshots WHERE id=$1)
		  AND NOT EXISTS (
			SELECT 1 FROM parser_snapshots s
			WHERE s.data_source_id=ds.id AND s.status='quarantined'
		  )
		  AND ds.consecutive_failures=0`, id)
	return nil
}

func (r *ParserSnapshotRepository) PreviousPublished(
	ctx context.Context,
	sourceID string,
) (*domain.ParserSnapshot, error) {
	var current string
	if err := r.db.GetContext(ctx, &current,
		`SELECT COALESCE(current_snapshot_id, '') FROM data_sources WHERE id=$1`,
		sourceID); err != nil {
		return nil, err
	}
	var row parserSnapshotRow
	err := r.db.GetContext(ctx, &row, `
		SELECT `+parserSnapshotColumns+`
		FROM parser_snapshots
		WHERE data_source_id=$1 AND status='published' AND id<>$2
		ORDER BY published_at DESC NULLS LAST, created_at DESC
		LIMIT 1`, sourceID, current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load previous parser snapshot: %w", err)
	}
	return decodeParserSnapshot(row)
}

func (r *ParserSnapshotRepository) Prune(ctx context.Context, sourceID, keepID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM parser_snapshots
		WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY status ORDER BY created_at DESC
				) AS position
				FROM parser_snapshots
				WHERE data_source_id=$1 AND id<>$2 AND status<>'quarantined'
			) ranked WHERE position > 20
		)`, sourceID, keepID)
	if err != nil {
		return fmt.Errorf("prune parser snapshots: %w", err)
	}
	return nil
}

func decodeParserSnapshot(row parserSnapshotRow) (*domain.ParserSnapshot, error) {
	result := &domain.ParserSnapshot{
		ID:           row.ID,
		DataSourceID: row.DataSourceID,
		ParseLogID:   row.ParseLogID,
		Status:       row.Status,
		Publishable:  row.Publishable,
		GroupCount:   row.GroupCount,
		LessonCount:  row.LessonCount,
		ReviewedBy:   row.ReviewedBy,
		ReviewNote:   row.ReviewNote,
		CreatedAt:    row.CreatedAt,
		PublishedAt:  row.PublishedAt,
		ReviewedAt:   row.ReviewedAt,
	}
	if err := json.Unmarshal(row.ReasonsRaw, &result.AnomalyReasons); err != nil {
		return nil, fmt.Errorf("decode snapshot %s anomalies: %w", row.ID, err)
	}
	if len(row.PayloadRaw) > 0 {
		if err := json.Unmarshal(row.PayloadRaw, &result.Payload); err != nil {
			return nil, fmt.Errorf("decode snapshot %s payload: %w", row.ID, err)
		}
	}
	return result, nil
}
