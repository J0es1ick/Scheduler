package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/jmoiron/sqlx"
)

type lessonSourceIdentity struct {
	LessonID     string     `db:"lesson_id"`
	UniversityID string     `db:"university_id"`
	SemesterID   string     `db:"semester_id"`
	SourceID     string     `db:"source_id"`
	ExternalID   string     `db:"external_id"`
	GroupID      string     `db:"group_id"`
	DayOfWeek    int        `db:"day_of_week"`
	SpecialDate  *time.Time `db:"special_date"`
	TimeStart    string     `db:"time_start"`
	WeekType     string     `db:"week_type"`
	Subgroup     int        `db:"subgroup"`
	Recurrence   []byte     `db:"recurrence"`
	IdentityKey  string     `db:"identity_key"`
}

func reconcileLessonIdentities(
	ctx context.Context,
	tx *sqlx.Tx,
	payload domain.ScheduleSnapshot,
	defaultSourceID string,
) (domain.ScheduleSnapshot, error) {
	var catalog []lessonSourceIdentity
	if err := tx.SelectContext(ctx, &catalog, `
		SELECT identity.lesson_id, identity.university_id, identity.semester_id,
		       identity.source_id, identity.external_id, identity.identity_key,
		       '' AS group_id, 0 AS day_of_week, NULL::date AS special_date,
		       '' AS time_start, '' AS week_type, 0 AS subgroup, '{}'::jsonb AS recurrence
		FROM lesson_source_identities identity
		WHERE identity.university_id=$1 AND identity.semester_id=$2
		UNION ALL
		SELECT lesson.id, lesson.university_id, lesson.semester_id,
		       COALESCE(lesson.source_id, ''), lesson.external_id, '', lesson.group_id,
		       COALESCE(lesson.day_of_week, 0), lesson.special_date,
		       lesson.time_start, lesson.week_type::text, lesson.subgroup, lesson.recurrence
		FROM lessons lesson
		WHERE lesson.university_id=$1 AND lesson.semester_id=$2
		  AND NOT EXISTS (
			SELECT 1 FROM lesson_source_identities identity
			WHERE identity.lesson_id=lesson.id
		  )`, payload.UniversityID, payload.SemesterID); err != nil {
		return payload, fmt.Errorf("publish snapshot: load lesson identities: %w", err)
	}

	byExternal := make(map[string]string)
	bySchedule := make(map[string]string)
	for _, identity := range catalog {
		if identity.IdentityKey == "" {
			var recurrence domain.RecurrenceRule
			_ = json.Unmarshal(identity.Recurrence, &recurrence)
			identity.IdentityKey = lessonIdentityKey(domain.Lesson{
				SemesterID: identity.SemesterID, GroupID: identity.GroupID,
				DayOfWeek: identity.DayOfWeek, SpecialDate: identity.SpecialDate,
				TimeStart: identity.TimeStart, WeekType: domain.WeekType(identity.WeekType),
				Subgroup: identity.Subgroup, Recurrence: recurrence,
			})
		}
		addUniqueIdentity(bySchedule, identity.IdentityKey, identity.LessonID)
		if identity.SourceID != "" && identity.ExternalID != "" {
			addUniqueIdentity(byExternal, identity.SourceID+"\x00"+identity.ExternalID, identity.LessonID)
		}
	}

	used := make(map[string]bool)
	for groupIndex := range payload.Groups {
		group := &payload.Groups[groupIndex]
		for lessonIndex := range group.Lessons {
			lesson := &group.Lessons[lessonIndex]
			if lesson.SourceID == "" {
				lesson.SourceID = defaultSourceID
			}
			candidate := ""
			if lesson.ExternalID != "" {
				candidate = byExternal[lesson.SourceID+"\x00"+lesson.ExternalID]
			}
			if candidate == "" {
				candidate = bySchedule[lessonIdentityKey(*lesson)]
			}
			if candidate != "" && !used[candidate] {
				lesson.ID = candidate
			}
			if used[lesson.ID] {
				return payload, fmt.Errorf("publish snapshot: duplicate reconciled lesson id %s", lesson.ID)
			}
			used[lesson.ID] = true
		}
	}
	return payload, nil
}

func storeLessonIdentities(ctx context.Context, tx *sqlx.Tx, payload domain.ScheduleSnapshot) error {
	type identityRecord struct {
		lessonID, universityID, semesterID, sourceID, externalID, identityKey string
	}
	records := make([]identityRecord, 0)
	for _, group := range payload.Groups {
		for _, lesson := range group.Lessons {
			records = append(records, identityRecord{
				lesson.ID, lesson.UniversityID, payload.SemesterID,
				lesson.SourceID, lesson.ExternalID, lessonIdentityKey(lesson),
			})
		}
	}
	const batchSize = 1000
	for start := 0; start < len(records); start += batchSize {
		end := min(start+batchSize, len(records))
		var query strings.Builder
		query.WriteString(`INSERT INTO lesson_source_identities (
			lesson_id, university_id, semester_id, source_id,
			external_id, identity_key, first_seen_at, last_seen_at
		) VALUES `)
		args := make([]any, 0, (end-start)*6)
		for index, record := range records[start:end] {
			if index > 0 {
				query.WriteByte(',')
			}
			base := index * 6
			fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,NOW(),NOW())",
				base+1, base+2, base+3, base+4, base+5, base+6)
			args = append(args, record.lessonID, record.universityID, record.semesterID,
				record.sourceID, record.externalID, record.identityKey)
		}
		query.WriteString(` ON CONFLICT (lesson_id) DO UPDATE SET
			university_id=EXCLUDED.university_id,
			semester_id=EXCLUDED.semester_id,
			source_id=EXCLUDED.source_id,
			external_id=EXCLUDED.external_id,
			identity_key=EXCLUDED.identity_key,
			last_seen_at=NOW()`)
		if _, err := tx.ExecContext(ctx, query.String(), args...); err != nil {
			return fmt.Errorf("publish snapshot: store lesson identity batch %d-%d: %w", start, end, err)
		}
	}
	return nil
}

func lessonIdentityKey(lesson domain.Lesson) string {
	specialDate := ""
	if lesson.SpecialDate != nil {
		specialDate = lesson.SpecialDate.Format(time.DateOnly)
	}
	recurrence, _ := json.Marshal(lesson.Recurrence)
	return fmt.Sprintf(
		"%s|%s|%d|%s|%s|%s|%d|%s",
		lesson.SemesterID, lesson.GroupID, lesson.DayOfWeek, specialDate,
		lesson.TimeStart, lesson.WeekType, lesson.Subgroup, recurrence,
	)
}

func addUniqueIdentity(index map[string]string, key, lessonID string) {
	if key == "" {
		return
	}
	if current, exists := index[key]; exists && current != lessonID {
		index[key] = ""
		return
	}
	index[key] = lessonID
}
