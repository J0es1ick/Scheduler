package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Store struct {
	db *sqlx.DB
}

func NewStore(db *sqlx.DB) *Store { return &Store{db: db} }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) Dashboard(ctx context.Context) (*Dashboard, error) {
	var result Dashboard
	if err := s.db.GetContext(ctx, &result.Stats, `
		SELECT
			(SELECT COUNT(*) FROM universities WHERE is_active) AS universities,
			(SELECT COUNT(*) FROM groups WHERE is_active) AS groups,
			(SELECT COUNT(*) FROM effective_lessons l JOIN groups g ON g.id=l.group_id WHERE g.is_active) AS lessons,
			(SELECT COUNT(*) FROM users) AS users,
			(SELECT COUNT(*) FROM subscriptions) AS subscriptions,
			COALESCE((SELECT ROUND(100.0 * COUNT(*) FILTER (WHERE status='success') / NULLIF(COUNT(*), 0), 1)
				FROM parse_logs WHERE started_at >= NOW() - INTERVAL '7 days'), 100) AS success_rate`); err != nil {
		return nil, fmt.Errorf("admin dashboard stats: %w", err)
	}
	var err error
	if result.Sources, err = s.Sources(ctx); err != nil {
		return nil, err
	}
	if result.RecentLogs, err = s.Logs(ctx, 8, "", ""); err != nil {
		return nil, err
	}
	if err = s.db.SelectContext(ctx, &result.Trend, `
		WITH days AS (
			SELECT generate_series(CURRENT_DATE - INTERVAL '6 days', CURRENT_DATE, INTERVAL '1 day')::date AS date
		)
		SELECT d.date,
			COALESCE(SUM(p.records_fetched), 0)::int AS records,
			COUNT(p.id) FILTER (WHERE p.status='success')::int AS success,
			COUNT(p.id) FILTER (WHERE p.status='failed')::int AS failed
		FROM days d
		LEFT JOIN parse_logs p ON p.started_at::date=d.date
		GROUP BY d.date ORDER BY d.date`); err != nil {
		return nil, fmt.Errorf("admin dashboard trend: %w", err)
	}
	if err = s.db.SelectContext(ctx, &result.Universities, `
		SELECT u.id, u.name,
			COUNT(DISTINCT g.id) FILTER (WHERE g.is_active)::int AS groups,
			COUNT(l.id) FILTER (WHERE g.is_active)::int AS lessons
		FROM universities u
		LEFT JOIN groups g ON g.university_id=u.id
		LEFT JOIN effective_lessons l ON l.group_id=g.id
		WHERE u.is_active
		GROUP BY u.id, u.name ORDER BY u.name`); err != nil {
		return nil, fmt.Errorf("admin university breakdown: %w", err)
	}
	return &result, nil
}

func (s *Store) Sources(ctx context.Context) ([]SourceView, error) {
	var sources []SourceView
	err := s.db.SelectContext(ctx, &sources, `
		SELECT ds.id, ds.university_id, u.name AS university_name,
			COALESCE(u.full_name, '') AS university_full_name,
			COALESCE(u.schedule_url, '') AS schedule_url,
			ds.adapter_type, ds.update_interval, ds.last_run_at,
			COALESCE(ds.last_error, '') AS last_error,
			COALESCE(latest.status::text, '') AS latest_status,
			latest.started_at AS latest_started_at,
			latest.finished_at AS latest_finished_at,
			COALESCE(latest.records_fetched, 0) AS latest_records,
			(SELECT COUNT(*) FROM groups g WHERE g.university_id=ds.university_id AND g.is_active)::int AS group_count,
			(SELECT COUNT(*) FROM effective_lessons l JOIN groups g ON g.id=l.group_id
				WHERE l.university_id=ds.university_id AND g.is_active)::int AS lesson_count
		FROM data_sources ds
		JOIN universities u ON u.id=ds.university_id
		LEFT JOIN LATERAL (
			SELECT status, started_at, finished_at, records_fetched
			FROM parse_logs WHERE data_source_id=ds.id ORDER BY started_at DESC LIMIT 1
		) latest ON TRUE
		ORDER BY u.name`)
	if err != nil {
		return nil, fmt.Errorf("admin list sources: %w", err)
	}
	now := time.Now()
	for i := range sources {
		source := &sources[i]
		if source.LastRunAt != nil {
			next := source.LastRunAt.Add(time.Duration(source.UpdateInterval) * time.Second)
			source.NextRunAt = &next
		}
		source.Running = source.LatestStatus == "running" && source.LatestStartedAt != nil && now.Sub(*source.LatestStartedAt) < 2*time.Hour
		switch {
		case source.Running:
			source.Health = "running"
		case source.LastError != "":
			source.Health = "error"
		default:
			source.Health = "healthy"
		}
	}
	return sources, nil
}

func (s *Store) Logs(ctx context.Context, limit int, sourceID, status string) ([]ParseLogView, error) {
	limit = clamp(limit, 1, 250)
	where := []string{"TRUE"}
	args := []any{}
	if sourceID != "" {
		args = append(args, sourceID)
		where = append(where, fmt.Sprintf("p.data_source_id=$%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("p.status::text=$%d", len(args)))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT p.id, p.data_source_id, u.name AS university_name, p.started_at,
			p.finished_at, p.status::text AS status, p.records_fetched,
			COALESCE(p.error_message, '') AS error_message,
			COALESCE((EXTRACT(EPOCH FROM (COALESCE(p.finished_at, NOW()) - p.started_at)) * 1000)::bigint, 0) AS duration_ms
		FROM parse_logs p
		JOIN data_sources ds ON ds.id=p.data_source_id
		JOIN universities u ON u.id=ds.university_id
		WHERE %s ORDER BY p.started_at DESC LIMIT $%d`, strings.Join(where, " AND "), len(args))
	var logs []ParseLogView
	if err := s.db.SelectContext(ctx, &logs, query, args...); err != nil {
		return nil, fmt.Errorf("admin list logs: %w", err)
	}
	return logs, nil
}

func (s *Store) Universities(ctx context.Context) ([]UniversityOption, error) {
	var result []UniversityOption
	if err := s.db.SelectContext(ctx, &result, `
		SELECT id, name, COALESCE(full_name, '') AS full_name,
			COALESCE(schedule_url, '') AS schedule_url, is_active
		FROM universities ORDER BY name`); err != nil {
		return nil, fmt.Errorf("admin list universities: %w", err)
	}
	return result, nil
}

func (s *Store) Groups(ctx context.Context, page, pageSize int, universityID, queryText string, selector bool) (*Page[GroupView], error) {
	page = clamp(page, 1, 100000)
	pageSize = clamp(pageSize, 10, 100)
	where := []string{"TRUE"}
	args := []any{}
	if universityID != "" {
		args = append(args, universityID)
		where = append(where, fmt.Sprintf("g.university_id=$%d", len(args)))
	}
	if queryText != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(queryText)
		args = append(args, "%"+escaped+"%")
		where = append(where, fmt.Sprintf("(g.name ILIKE $%d ESCAPE '\\' OR g.id ILIKE $%d ESCAPE '\\')", len(args), len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM groups g WHERE "+whereSQL, args...); err != nil {
		return nil, fmt.Errorf("admin count groups: %w", err)
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	orderBy := `g.is_active DESC, u.name,
		CASE WHEN g.name ~ '^[0-9]+' THEN substring(g.name from '^[0-9]+')::int ELSE 2147483647 END,
		g.name`
	if selector {
		// Interleave the first matches from each course. Otherwise a short selector is
		// filled by first-year groups before the user has typed a course number.
		orderBy = `g.is_active DESC,
			CASE WHEN g.name ~ '^[0-9]+' THEN 0 ELSE 1 END,
			ROW_NUMBER() OVER (
				PARTITION BY g.university_id, COALESCE(substring(g.name from '^[0-9]+'), '__other__')
				ORDER BY length(g.name), g.name
			),
			u.name,
			CASE WHEN g.name ~ '^[0-9]+' THEN substring(g.name from '^[0-9]+')::int ELSE 2147483647 END,
			g.name`
	}
	rowsQuery := fmt.Sprintf(`
		SELECT g.id, g.name, g.university_id, u.name AS university_name,
			g.is_active,
			(SELECT COUNT(*)::int FROM effective_lessons l WHERE l.group_id=g.id) AS lesson_count,
			g.updated_at
		FROM groups g JOIN universities u ON u.id=g.university_id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`, whereSQL, orderBy, len(args)+1, len(args)+2)
	items := []GroupView{}
	if err := s.db.SelectContext(ctx, &items, rowsQuery, queryArgs...); err != nil {
		return nil, fmt.Errorf("admin list groups: %w", err)
	}
	return &Page[GroupView]{Items: items, Pagination: Pagination{Page: page, PageSize: pageSize, Total: total}}, nil
}

func (s *Store) Lessons(ctx context.Context, page, pageSize int, universityID, groupID, queryText string) (*Page[LessonView], error) {
	page = clamp(page, 1, 100000)
	pageSize = clamp(pageSize, 10, 100)
	where := []string{"TRUE"}
	args := []any{}
	if universityID != "" {
		args = append(args, universityID)
		where = append(where, fmt.Sprintf("l.university_id=$%d", len(args)))
	}
	if groupID != "" {
		args = append(args, groupID)
		where = append(where, fmt.Sprintf("l.group_id=$%d", len(args)))
	}
	if queryText != "" {
		args = append(args, "%"+queryText+"%")
		position := len(args)
		where = append(where, fmt.Sprintf("(l.subject ILIKE $%d OR l.teacher ILIKE $%d OR l.room ILIKE $%d OR g.name ILIKE $%d)", position, position, position, position))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	countQuery := `SELECT COUNT(*) FROM effective_lessons l JOIN groups g ON g.id=l.group_id WHERE ` + whereSQL
	if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, fmt.Errorf("admin count lessons: %w", err)
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rowsQuery := fmt.Sprintf(`
		SELECT l.id, u.name AS university_name, l.group_id, g.name AS group_name,
			l.subject, l.type::text AS type, l.teacher, l.room,
			COALESCE(l.day_of_week, 0) AS day_of_week, l.special_date,
			l.time_start, l.time_end, l.week_type::text AS week_type, l.subgroup,
			l.valid_from, l.valid_to
		FROM effective_lessons l JOIN groups g ON g.id=l.group_id
		JOIN universities u ON u.id=l.university_id
		WHERE %s
		ORDER BY COALESCE(l.special_date, l.valid_from), g.name, l.day_of_week, l.time_start
		LIMIT $%d OFFSET $%d`, whereSQL, len(args)+1, len(args)+2)
	items := []LessonView{}
	if err := s.db.SelectContext(ctx, &items, rowsQuery, queryArgs...); err != nil {
		return nil, fmt.Errorf("admin list lessons: %w", err)
	}
	return &Page[LessonView]{Items: items, Pagination: Pagination{Page: page, PageSize: pageSize, Total: total}}, nil
}

func (s *Store) Users(ctx context.Context, queryText string, limit int) ([]UserView, error) {
	limit = clamp(limit, 1, 250)
	args := []any{}
	where := "TRUE"
	if queryText != "" {
		args = append(args, "%"+queryText+"%")
		where = `(u.id ILIKE $1 OR u.username ILIKE $1)`
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT u.id, COALESCE(u.username, '') AS username, u.is_admin,
			COUNT(s.id)::int AS subscriptions,
			COALESCE(u.default_group_id, '') AS default_group_id,
			COALESCE(dg.name, '') AS default_group_name,
			u.notifications_enabled, u.created_at, u.updated_at
		FROM users u
		LEFT JOIN subscriptions s ON s.user_id=u.id
		LEFT JOIN groups dg ON dg.id=u.default_group_id
		WHERE %s
		GROUP BY u.id, u.username, u.is_admin, u.default_group_id,
			dg.name, u.notifications_enabled, u.created_at, u.updated_at
		ORDER BY u.is_admin DESC, u.updated_at DESC LIMIT $%d`, where, len(args))
	var users []UserView
	if err := s.db.SelectContext(ctx, &users, query, args...); err != nil {
		return nil, fmt.Errorf("admin list users: %w", err)
	}
	return users, nil
}

func (s *Store) UpdateSourceInterval(ctx context.Context, sourceID string, interval int) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE data_sources SET update_interval=$1, updated_at=NOW() WHERE id=$2`, interval, sourceID)
	if err != nil {
		return fmt.Errorf("admin update source interval: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateUserAdmin(ctx context.Context, userID string, isAdmin bool) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET is_admin=$1, updated_at=NOW() WHERE id=$2`, isAdmin, userID)
	if err != nil {
		return fmt.Errorf("admin update user role: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TelegramAdmin(ctx context.Context, userID string) (*UserView, error) {
	var user UserView
	err := s.db.GetContext(ctx, &user, `
		SELECT u.id, COALESCE(u.username, '') AS username, u.is_admin,
			0 AS subscriptions,
			COALESCE(u.default_group_id, '') AS default_group_id,
			COALESCE(g.name, '') AS default_group_name,
			u.notifications_enabled, u.created_at, u.updated_at
		FROM users u
		LEFT JOIN groups g ON g.id=u.default_group_id
		WHERE u.id=$1`, userID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) SupportRequests(ctx context.Context, status, requestType, queryText string, limit int) ([]SupportRequestView, error) {
	limit = clamp(limit, 1, 250)
	where := []string{"TRUE"}
	args := make([]any, 0, 4)
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("r.status=$%d", len(args)))
	}
	if requestType != "" {
		args = append(args, requestType)
		where = append(where, fmt.Sprintf("r.request_type=$%d", len(args)))
	}
	if queryText != "" {
		args = append(args, "%"+queryText+"%")
		position := len(args)
		where = append(where, fmt.Sprintf(
			"(r.id ILIKE $%d OR r.details ILIKE $%d OR u.id ILIKE $%d OR u.username ILIKE $%d)",
			position, position, position, position,
		))
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
		SELECT r.id, r.user_id, COALESCE(u.username, '') AS username,
			r.request_type, r.details, r.status, r.review_note,
			r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at
		FROM support_requests r
		JOIN users u ON u.id=r.user_id
		WHERE %s
		ORDER BY CASE r.status WHEN 'pending' THEN 0 ELSE 1 END, r.created_at DESC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))
	var items []SupportRequestView
	if err := s.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, fmt.Errorf("admin list support requests: %w", err)
	}
	if items == nil {
		items = []SupportRequestView{}
	}
	return items, nil
}

func (s *Store) ResolveSupportRequest(
	ctx context.Context,
	requestID, status, reviewNote, actorID string,
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("resolve support request: begin: %w", err)
	}
	defer tx.Rollback()

	var current struct {
		UserID string `db:"user_id"`
		Status string `db:"status"`
	}
	err = tx.GetContext(ctx, &current, `
		SELECT user_id, status FROM support_requests WHERE id=$1 FOR UPDATE`, requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve support request: load: %w", err)
	}
	if current.Status != "pending" {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE support_requests
		SET status=$2, review_note=$3, reviewed_by=$4,
			reviewed_at=NOW(), updated_at=NOW()
		WHERE id=$1`, requestID, status, reviewNote, actorID); err != nil {
		return fmt.Errorf("resolve support request: update: %w", err)
	}

	message := fmt.Sprintf("✅ Обращение %s принято в работу.", requestID)
	if status == "rejected" {
		message = fmt.Sprintf("Обращение %s отклонено.", requestID)
	}
	if reviewNote != "" {
		message += "\n\nКомментарий администратора:\n" + reviewNote
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO bot_outbox (id, user_id, request_id, kind, body)
		VALUES ($1, $2, $3, 'support_resolution', $4)`,
		requestID+":resolution", current.UserID, requestID, message); err != nil {
		return fmt.Errorf("resolve support request: enqueue response: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("resolve support request: commit: %w", err)
	}
	return nil
}

func (s *Store) WriteAudit(ctx context.Context, actor AdminIdentity, action, objectType, objectID string, details any, ipAddress string) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("admin marshal audit details: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO admin_audit_logs
			(id, actor_id, actor_name, action, object_type, object_id, details, ip_address, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,NOW())`,
		uuid.NewString(), actor.ID, actor.Name, action, objectType, objectID, payload, ipAddress)
	if err != nil {
		return fmt.Errorf("admin write audit: %w", err)
	}
	return nil
}

func (s *Store) AuditLogs(ctx context.Context, limit int) ([]AuditLogView, error) {
	limit = clamp(limit, 1, 250)
	var logs []AuditLogView
	if err := s.db.SelectContext(ctx, &logs, `
		SELECT id, actor_id, actor_name, action, object_type, object_id,
			details, ip_address, created_at
		FROM admin_audit_logs ORDER BY created_at DESC LIMIT $1`, limit); err != nil {
		return nil, fmt.Errorf("admin list audit logs: %w", err)
	}
	for i := range logs {
		logs[i].Details = make(map[string]any)
		_ = json.Unmarshal(logs[i].DetailsRaw, &logs[i].Details)
	}
	return logs, nil
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
