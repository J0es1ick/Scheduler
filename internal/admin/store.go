package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Store struct {
	db *sqlx.DB
}

var ErrSourceBusy = errors.New("data source is currently running")

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
	if result.Sources, err = s.Sources(ctx, false); err != nil {
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
	operations, err := s.OperationalHealth(ctx)
	if err != nil {
		return nil, err
	}
	result.Operations = *operations
	return &result, nil
}

func (s *Store) Sources(ctx context.Context, includeArchived bool) ([]SourceView, error) {
	var sources []SourceView
	err := s.db.SelectContext(ctx, &sources, `
		SELECT ds.id, ds.university_id, u.name AS university_name,
			COALESCE(u.full_name, '') AS university_full_name,
			COALESCE(u.schedule_url, '') AS schedule_url,
			ds.adapter_type, ds.lifecycle_status, ds.archived_at, ds.is_enabled, ds.update_interval, ds.last_run_at, ds.last_success_at,
			COALESCE((ds.quality_policy->>'allow_empty')::boolean, FALSE) AS allow_empty,
			(COALESCE(u.schedule_url, '') LIKE 'http://%') AS insecure_transport,
			COALESCE(ds.last_error, '') AS last_error,
			ds.consecutive_failures, ds.next_retry_at,
			COALESCE(ds.current_snapshot_id, '') AS current_snapshot_id,
			(SELECT COUNT(*)::int FROM parser_snapshots ps
				WHERE ps.data_source_id=ds.id AND ps.status='quarantined') AS quarantined_count,
			COALESCE(latest.status::text, '') AS latest_status,
			latest.started_at AS latest_started_at,
			latest.finished_at AS latest_finished_at,
			COALESCE(latest.records_fetched, 0) AS latest_records,
			(SELECT COUNT(*) FROM groups g WHERE g.university_id=ds.university_id AND g.is_active)::int AS group_count,
			(SELECT COUNT(*) FROM effective_lessons l JOIN groups g ON g.id=l.group_id
				WHERE l.university_id=ds.university_id AND g.is_active)::int AS lesson_count,
			COALESCE(diagnostic.id, '') AS diagnostic_id,
			COALESCE(diagnostic.category, '') AS diagnostic_category,
			COALESCE(diagnostic.summary, '') AS diagnostic_summary,
			COALESCE(diagnostic.group_id, '') AS diagnostic_group_id,
			COALESCE(diagnostic.http_status, 0) AS diagnostic_http_status,
			COALESCE(diagnostic.content_type, '') AS diagnostic_content_type,
			COALESCE(diagnostic.response_size, 0) AS diagnostic_response_size,
			COALESCE(diagnostic.response_sha256, '') AS diagnostic_response_sha256,
			COALESCE(diagnostic.response_preview, '') AS diagnostic_response_preview,
			COALESCE(diagnostic.occurrences, 0) AS diagnostic_occurrences,
			diagnostic.created_at AS diagnostic_created_at
		FROM data_sources ds
		JOIN universities u ON u.id=ds.university_id
		LEFT JOIN LATERAL (
			SELECT status, started_at, finished_at, records_fetched
			FROM parse_logs WHERE data_source_id=ds.id ORDER BY started_at DESC LIMIT 1
		) latest ON TRUE
		LEFT JOIN LATERAL (
			SELECT id, category, summary, group_id, http_status, content_type,
				response_size, response_sha256, response_preview,
				occurrences, created_at
			FROM parser_diagnostics
			WHERE data_source_id=ds.id
			ORDER BY created_at DESC
			LIMIT 1
		) diagnostic ON TRUE
		WHERE $1 OR ds.lifecycle_status<>'archived'
		ORDER BY ds.lifecycle_status='archived', u.name`, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("admin list sources: %w", err)
	}
	now := time.Now()
	for i := range sources {
		source := &sources[i]
		source.LastError = compactParserError(source.LastError)
		if !source.IsEnabled {
			source.NextRunAt = nil
		} else if source.LastError != "" && source.NextRetryAt != nil {
			source.NextRunAt = source.NextRetryAt
		} else if source.LastRunAt != nil {
			next := source.LastRunAt.Add(time.Duration(source.UpdateInterval) * time.Second)
			source.NextRunAt = &next
		}
		source.Running = source.LatestStatus == "running" && source.LatestStartedAt != nil && now.Sub(*source.LatestStartedAt) < 2*time.Hour
		switch {
		case !source.IsEnabled:
			source.Health = "disabled"
		case source.Running:
			source.Health = "running"
		case source.QuarantinedCount > 0:
			source.Health = "quarantined"
		case source.LastError != "":
			source.Health = "error"
		case source.LessonCount == 0 && !source.AllowEmpty:
			source.Health = "empty"
		case source.LastSuccessAt == nil ||
			now.Sub(*source.LastSuccessAt) > 2*time.Duration(source.UpdateInterval)*time.Second+5*time.Minute:
			source.Health = "stale"
		default:
			source.Health = "healthy"
		}
	}
	return sources, nil
}

func (s *Store) OperationalHealth(ctx context.Context) (*OperationalHealth, error) {
	result := &OperationalHealth{Database: true, CheckedAt: time.Now()}
	sources, err := s.Sources(ctx, false)
	if err != nil {
		return nil, err
	}
	result.SourcesTotal = len(sources)
	for _, source := range sources {
		switch source.Health {
		case "healthy":
			result.SourcesHealthy++
		case "running":
			result.SourcesRunning++
		case "stale":
			result.SourcesStale++
		case "quarantined":
			result.SourcesQuarantined++
		case "disabled":
			result.SourcesDisabled++
		default:
			result.SourcesError++
		}
	}
	if err := s.db.GetContext(ctx, result, `
		SELECT
			(SELECT COUNT(*)::int FROM notification_deliveries WHERE status='pending') AS pending_notifications,
			(SELECT COUNT(*)::int FROM notification_deliveries WHERE status='failed') AS failed_notifications,
			(SELECT COUNT(*)::int FROM bot_outbox WHERE status='pending') AS pending_outbox,
			(SELECT COUNT(*)::int FROM bot_outbox WHERE status='failed') AS failed_outbox,
			(SELECT COUNT(*)::int FROM connector_ingestion_runs WHERE status IN ('received','processing')) AS pending_connector_runs,
			(SELECT COUNT(*)::int FROM connector_ingestion_runs WHERE status='failed') AS failed_connector_runs,
			COALESCE((
				SELECT EXTRACT(EPOCH FROM (NOW()-MIN(created_at)))::bigint
				FROM (
					SELECT created_at FROM notification_deliveries WHERE status='pending'
					UNION ALL
					SELECT created_at FROM bot_outbox WHERE status='pending'
				) pending
			), 0) AS oldest_pending_seconds,
			(SELECT MAX(finished_at) FROM parse_logs WHERE status='success') AS last_successful_parse_at`,
	); err != nil {
		return nil, fmt.Errorf("load operational health: %w", err)
	}
	workerStatus, err := repository.NewWorkerStatusRepository(s.db).Get(
		ctx,
		domain.LessonReminderWorker,
	)
	if err != nil {
		return nil, fmt.Errorf("load reminder worker health: %w", err)
	}
	result.ReminderWorker = *workerStatus
	result.Status = "healthy"
	if result.SourcesStale > 0 || result.SourcesError > 0 ||
		result.SourcesQuarantined > 0 || result.FailedNotifications > 0 ||
		result.FailedOutbox > 0 || result.FailedConnectorRuns > 0 || result.OldestPendingSeconds > 300 ||
		result.ReminderWorker.LastError != "" ||
		result.ReminderWorker.LastFinishedAt == nil ||
		result.CheckedAt.Sub(*result.ReminderWorker.LastFinishedAt) > 3*time.Minute {
		result.Status = "degraded"
	}
	return result, nil
}

func (s *Store) ParserSnapshots(
	ctx context.Context,
	sourceID, status string,
	limit int,
) ([]domain.ParserSnapshot, error) {
	return repository.NewParserSnapshotRepository(s.db).List(ctx, sourceID, status, limit)
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
	for index := range logs {
		logs[index].ErrorMessage = compactParserError(logs[index].ErrorMessage)
	}
	return logs, nil
}

func compactParserError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	if marker := strings.Index(message, ": group "); marker >= 0 {
		message = message[:marker] +
			". Подробные ответы сохранены в диагностике источника."
	}
	const maximum = 700
	runes := []rune(message)
	if len(runes) > maximum {
		message = string(runes[:maximum]) +
			"… Подробные ответы сохранены в диагностике источника."
	}
	return message
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
		SELECT u.id, COALESCE(u.username, '') AS username, u.is_admin, u.admin_role,
			COUNT(s.id)::int AS subscriptions,
			COALESCE(u.default_group_id, '') AS default_group_id,
			COALESCE(dg.name, '') AS default_group_name,
			u.notifications_enabled, u.created_at, u.updated_at
		FROM users u
		LEFT JOIN subscriptions s ON s.user_id=u.id
		LEFT JOIN groups dg ON dg.id=u.default_group_id
		WHERE %s
		GROUP BY u.id, u.username, u.is_admin, u.admin_role, u.default_group_id,
			dg.name, u.notifications_enabled, u.created_at, u.updated_at
		ORDER BY u.is_admin DESC, u.updated_at DESC LIMIT $%d`, where, len(args))
	var users []UserView
	if err := s.db.SelectContext(ctx, &users, query, args...); err != nil {
		return nil, fmt.Errorf("admin list users: %w", err)
	}
	return users, nil
}

func (s *Store) UpdateSourceSettings(
	ctx context.Context,
	sourceID string,
	interval *int,
	isEnabled *bool,
) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE data_sources
		 SET update_interval=COALESCE($1::int, update_interval),
		     last_run_at=CASE
		       WHEN $2::boolean IS TRUE AND NOT is_enabled THEN NULL
		       ELSE last_run_at
		     END,
		     next_retry_at=CASE
		       WHEN $2::boolean IS TRUE AND NOT is_enabled THEN NULL
		       ELSE next_retry_at
		     END,
		     is_enabled=COALESCE($2::boolean, is_enabled),
		     updated_at=NOW()
		 WHERE id=$3`, interval, isEnabled, sourceID)
	if err != nil {
		return fmt.Errorf("admin update source settings: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SourceEnabled(ctx context.Context, sourceID string) (bool, error) {
	var enabled bool
	if err := s.db.GetContext(ctx, &enabled,
		`SELECT is_enabled FROM data_sources WHERE id=$1`, sourceID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("admin get source state: %w", err)
	}
	return enabled, nil
}

func (s *Store) DeleteSource(ctx context.Context, sourceID string) error {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("admin delete source: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var acquired bool
	if err = tx.GetContext(ctx, &acquired,
		`SELECT pg_try_advisory_xact_lock(hashtext('scheduler-parser'), hashtext($1))`,
		sourceID,
	); err != nil {
		return fmt.Errorf("admin delete source: acquire lock: %w", err)
	}
	if !acquired {
		return ErrSourceBusy
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE data_sources
		SET lifecycle_status='archived', is_enabled=FALSE,
			archived_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND lifecycle_status<>'archived'`, sourceID)
	if err != nil {
		return fmt.Errorf("admin delete source: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE connector_clients
		SET status='archived', updated_at=NOW()
		WHERE data_source_id=$1 AND status<>'archived'`, sourceID); err != nil {
		return fmt.Errorf("admin delete source: archive connector: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("admin delete source: commit: %w", err)
	}
	return nil
}

func (s *Store) RestoreSource(ctx context.Context, sourceID string) (string, error) {
	tx, err := s.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("admin restore source: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var acquired bool
	if err = tx.GetContext(ctx, &acquired,
		`SELECT pg_try_advisory_xact_lock(hashtext('scheduler-parser'), hashtext($1))`,
		sourceID,
	); err != nil {
		return "", fmt.Errorf("admin restore source: acquire lock: %w", err)
	}
	if !acquired {
		return "", ErrSourceBusy
	}

	var hasConnector bool
	if err = tx.GetContext(ctx, &hasConnector,
		`SELECT EXISTS(SELECT 1 FROM connector_clients WHERE data_source_id=$1)`, sourceID,
	); err != nil {
		return "", fmt.Errorf("admin restore source: detect connector: %w", err)
	}
	lifecycle := domain.ConnectorStatusActive
	if hasConnector {
		lifecycle = domain.ConnectorStatusDraft
		if _, err = tx.ExecContext(ctx, `
			UPDATE connector_clients SET status='draft', updated_at=NOW()
			WHERE data_source_id=$1`, sourceID); err != nil {
			return "", fmt.Errorf("admin restore source: restore connector: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE data_sources
		SET lifecycle_status=$2, is_enabled=FALSE, archived_at=NULL, updated_at=NOW()
		WHERE id=$1 AND lifecycle_status='archived'`, sourceID, lifecycle)
	if err != nil {
		if repository.IsActiveSourceConflict(err) {
			return "", repository.ErrActiveSourceConflict
		}
		return "", fmt.Errorf("admin restore source: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return "", ErrNotFound
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("admin restore source: commit: %w", err)
	}
	return lifecycle, nil
}

func (s *Store) UpdateUserAdmin(ctx context.Context, userID string, isAdmin bool) error {
	role := "none"
	if isAdmin {
		role = "owner"
	}
	return s.UpdateUserAdminRole(ctx, userID, role)
}

func (s *Store) UpdateUserAdminRole(ctx context.Context, userID, role string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("admin update user role: begin: %w", err)
	}
	defer tx.Rollback()
	var previous string
	if err = tx.GetContext(ctx, &previous, `SELECT admin_role FROM users WHERE id=$1 FOR UPDATE`, userID); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("admin load user role: %w", err)
	}
	if previous == "owner" && role != "owner" {
		var owners int
		if err = tx.GetContext(ctx, &owners, `SELECT COUNT(*)::int FROM users WHERE admin_role='owner'`); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrConflict
		}
	}
	isAdmin := role != "none"
	result, err := tx.ExecContext(ctx,
		`UPDATE users
		 SET is_admin=$1, admin_role=$2, telegram_menu_fingerprint='', updated_at=NOW()
		 WHERE id=$3`, isAdmin, role, userID)
	if err != nil {
		return fmt.Errorf("admin update user role: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) TelegramAdmin(ctx context.Context, userID string) (*UserView, error) {
	var user UserView
	err := s.db.GetContext(ctx, &user, `
		SELECT u.id, COALESCE(u.username, '') AS username, u.is_admin, u.admin_role,
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
